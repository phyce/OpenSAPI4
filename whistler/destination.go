package whistler

import (
	"bytes"
	"errors"
	"io"
)

// The destination's queue: the synthesizer waits once it holds queueCapacity messages, until the window
// thread has drained it to queueDrained (engine.md §12.2).
const (
	queueCapacity = 10
	queueDrained  = 2
)

// destination is the window thread, between the synthesizer and the sink. While the sink takes what the
// synthesizer posts, it gets it at once. When the sink stops taking samples, the window thread holds the chunk
// it didn't take and drains the queue up to it each time the synthesizer wakes it (the throttle rule), so the
// queue fills and the stages stop at their waits. A pause holds the next chunk of samples in the same way
// (engine.md §14): what the synthesizer posts from then on is kept, and each time the window thread drains the
// queue, the sink gets the events kept before the held chunk. The rest waits for Resume.
type destination struct {
	sink Sink
	// err is what stopped the sink taking samples: its error, or a frame's.
	err    error
	paused bool
	queued int
	full   bool
	chunks int
	// held counts the queued messages up to the chunk the sink didn't take, while that chunk is queued.
	held  int
	stuck bool
	// untaken is set while the chunk being made has samples the sink didn't take.
	untaken bool
	// kept is every post since the pause with what the sink is still to get of it, making what the
	// synthesizer has made for its next post, and pending the bytes of audio in both.
	kept    []keptPost
	making  []output
	pending int64
}

type keptPost struct {
	messages int
	chunk    bool
	outputs  []output
}

// holdsSamples reports whether the post is a chunk with samples still to give.
func (post *keptPost) holdsSamples() bool {
	for _, made := range post.outputs {
		if made.audio != nil {
			return true
		}
	}
	return false
}

// output is an event, or a piece of audio when event is nil.
type output struct {
	event *Event
	audio []byte
}

// take is the synthesizer taking an item: it wakes the window thread when the queue is full.
func (engine *Engine) take() {
	if engine.stages.destination.queued >= queueCapacity {
		engine.drain()
	}
}

// post puts messages on the queue, the last of them a chunk of samples when chunk is set. The window thread
// is woken after the fifth chunk of each text and when a chunk fills the queue (and by the synthesizer after a
// text's end).
func (engine *Engine) post(count int, chunk bool) {
	destination := &engine.stages.destination
	destination.queued += count
	if chunk && destination.untaken && destination.held == 0 && !destination.stuck {
		destination.held = destination.queued
	}
	destination.untaken = false
	if destination.paused {
		destination.kept = append(destination.kept, keptPost{messages: count, chunk: chunk, outputs: destination.making})
		destination.making = nil
	}
	if destination.queued >= queueCapacity {
		destination.full = true
	}
	if chunk {
		destination.chunks++
		if destination.chunks == 5 || destination.queued >= queueCapacity {
			engine.drain()
		}
	}
}

// drain is the window thread handing the queued messages to the sink, up to the chunk it doesn't take.
func (engine *Engine) drain() {
	destination := &engine.stages.destination
	switch {
	case destination.held > 0:
		destination.queued -= destination.held
		destination.held, destination.stuck = 0, true
		engine.handOut()
	case !destination.stuck:
		destination.queued = 0
		engine.handOut()
	}
	if destination.queued <= queueDrained {
		destination.full = false
	}
}

// handOut gives the sink the events kept before the held chunk's samples, its Visual included.
func (engine *Engine) handOut() {
	destination := &engine.stages.destination
	for len(destination.kept) > 0 {
		post := &destination.kept[0]
		for len(post.outputs) > 0 && post.outputs[0].audio == nil {
			if destination.err == nil {
				engine.deliver(post.outputs[0])
			}
			post.outputs = post.outputs[1:]
		}
		if len(post.outputs) > 0 {
			return
		}
		destination.kept = destination.kept[1:]
	}
}

func (engine *Engine) event(event Event) {
	event.Sample = (engine.byteCount + engine.stages.destination.pending) / int64(engine.format.Bits/8)
	event.Text = engine.stages.synthesizer.text
	engine.give(output{event: &event})
}

func (engine *Engine) audio(data []byte) {
	engine.give(output{audio: data})
}

// give hands what the synthesizer made to the sink, or keeps it while the engine is paused.
func (engine *Engine) give(made output) {
	destination := &engine.stages.destination
	if destination.err == nil && !destination.paused {
		made = engine.deliver(made)
		if len(made.audio) == 0 {
			return
		}
	}
	if made.audio == nil {
		if destination.err == nil {
			destination.making = append(destination.making, made)
		}
		return
	}
	destination.untaken = true
	if destination.err == nil {
		made.audio = bytes.Clone(made.audio)
		destination.pending += int64(len(made.audio))
		destination.making = append(destination.making, made)
	}
}

// deliver gives the sink an output and returns what it didn't take of it, which is nothing unless it stopped
// or paused.
func (engine *Engine) deliver(made output) output {
	destination := &engine.stages.destination
	var err error
	if made.event != nil {
		err = destination.sink.Event(*made.event)
		made = output{}
	} else {
		var taken int
		taken, err = destination.sink.Audio(made.audio)
		taken = min(max(taken, 0), len(made.audio))
		engine.byteCount += int64(taken)
		if err == nil && taken < len(made.audio) {
			err = io.ErrShortWrite
		}
		made.audio = made.audio[taken:]
	}
	switch {
	case err == nil:
	case errors.Is(err, ErrPaused):
		destination.paused = true
	default:
		destination.err = err
	}
	return made
}

// handOver is the window thread at Resume: it gives the sink the chunk it held and everything kept after it,
// and drains the queue. When the sink stops, or pauses and then reaches samples again, the chunk it is in is
// held, and the posts after that chunk stay queued.
func (engine *Engine) handOver() {
	destination := &engine.stages.destination
	kept := destination.kept
	destination.kept = nil
	for index := range kept {
		post := &kept[index]
		holding := false
		for len(post.outputs) > 0 && destination.err == nil {
			made := post.outputs[0]
			if made.audio != nil && destination.paused {
				holding = true
				break
			}
			destination.pending -= int64(len(made.audio))
			rest := engine.deliver(made)
			if len(rest.audio) > 0 && destination.err == nil {
				destination.pending += int64(len(rest.audio))
				post.outputs[0] = rest
				holding = true
				break
			}
			post.outputs = post.outputs[1:]
		}
		if holding || destination.err != nil {
			destination.requeue(kept[index:])
			return
		}
	}
	destination.queued, destination.held, destination.stuck, destination.full = 0, 0, false, false
}

// requeue leaves the queue as the window thread does when the sink stops taking from it in the first of posts:
// it holds the first chunk among them with samples to give, and the posts after that chunk stay queued.
func (destination *destination) requeue(posts []keptPost) {
	destination.kept = posts
	destination.queued, destination.held, destination.stuck = 0, 0, false
	for index := range posts {
		if posts[index].holdsSamples() {
			destination.stuck = true
			for _, queued := range posts[index+1:] {
				destination.queued += queued.messages
			}
			break
		}
	}
	if destination.queued <= queueDrained {
		destination.full = false
	}
}
