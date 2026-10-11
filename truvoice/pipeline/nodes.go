package pipeline

import "encoding/binary"

// NodeCount is the number of nodes in the pool.
const NodeCount = 618

// The node types, the low three bits of a node's flags (pipeline.md §2.1).
const (
	Control = iota
	Character
	Letter
	Phoneme
	Segment
	Generated
	Free
	Sentinel
)

const (
	listFirst = NodeCount + iota
	listLast
	freeFirst
	freeLast
	nodeSlots
)

const (
	typeBits     = 7
	valueOffset  = 4
	secondOffset = 13
	symbolOffset = 14
	lowFlags     = 0xff
)

// A Node is one of the pool's 32-byte records: Data is its bytes from offset 8 (the flags) to 0x1b, Text the
// text at offset 0x1c. Freeing a node changes only its type, and taking it again sets its type and symbol and
// clears only flag bits 3-7, its value, its second value and bytes 0x14 and 0x19 (pipeline.md §2.3): the rest
// is what its last use left.
type Node struct {
	Next, Previous *Node
	Data           [20]byte
	Text           *Text
	index          int
}

// A Text is a text the engine speaks. The pipeline only carries it, on the nodes of its bookmarks and words.
type Text struct {
	Number int
}

// Index is the node's place in the pool, or NodeCount and up for the sentinels.
func (node *Node) Index() int {
	return node.index
}

func (node *Node) Flags() uint32 {
	return binary.LittleEndian.Uint32(node.Data[:])
}

func (node *Node) SetFlags(flags uint32) {
	binary.LittleEndian.PutUint32(node.Data[:], flags)
}

func (node *Node) Type() int {
	return int(node.Data[0] & typeBits)
}

func (node *Node) Value() uint32 {
	return binary.LittleEndian.Uint32(node.Data[valueOffset:])
}

func (node *Node) SetValue(value uint32) {
	binary.LittleEndian.PutUint32(node.Data[valueOffset:], value)
}

func (node *Node) Second() byte {
	return node.Data[secondOffset]
}

func (node *Node) SetSecond(second byte) {
	node.Data[secondOffset] = second
}

func (node *Node) Symbol() byte {
	return node.Data[symbolOffset]
}

func (node *Node) SetSymbol(symbol byte) {
	node.Data[symbolOffset] = symbol
}

func (node *Node) unlink() {
	if node.Previous != nil {
		node.Previous.Next = node.Next
	}
	if node.Next != nil {
		node.Next.Previous = node.Previous
	}
}

func (node *Node) linkBefore(place *Node) {
	node.Previous = place.Previous
	place.Previous.Next = node
	node.Next = place
	place.Previous = node
}

// First and Last are the sentinels at the two ends of the list of nodes in use.
func (pipeline *Pipeline) First() *Node {
	return &pipeline.nodes[listFirst]
}

func (pipeline *Pipeline) Last() *Node {
	return &pipeline.nodes[listLast]
}

// Node returns the node at index, a pool index or a sentinel's.
func (pipeline *Pipeline) Node(index int) *Node {
	return &pipeline.nodes[index]
}

// ResetNodes puts every node on the free list in pool order, empties the list in use and the cursors, and
// gives the cursors the settings (pipeline.md §2.2).
func (pipeline *Pipeline) ResetNodes() {
	pipeline.Current = nil
	pipeline.Free = NodeCount
	first, last := &pipeline.nodes[freeFirst], &pipeline.nodes[freeLast]
	first.Previous = nil
	first.Data[0] |= typeBits
	last.Next = nil
	last.Data[0] |= typeBits
	previous := first
	for index := range NodeCount {
		node := &pipeline.nodes[index]
		previous.Next = node
		node.Previous = previous
		node.Data[0] = node.Data[0]&^1 | Free
		previous = node
	}
	previous.Next = last
	last.Previous = previous
	listFirst, listLast := pipeline.First(), pipeline.Last()
	listLast.Next = nil
	listLast.Previous = listFirst
	listLast.Data[0] |= typeBits
	listFirst.Next = listLast
	listFirst.Previous = nil
	listFirst.Data[0] |= typeBits
	for index := range pipeline.Cursors {
		cursor := &pipeline.Cursors[index]
		cursor.Start, cursor.Done, cursor.Scan, cursor.First, cursor.End = nil, nil, nil, nil, nil
		cursor.Working = [2]*Node{}
		cursor.Settings = pipeline.Settings
	}
}

// NewNode takes the first free node and links it into the list before at, or after it, as a node of the type
// with the symbol. The current cursor's window grows by the node when it is linked at either end (pipeline.md
// §2.3).
func (pipeline *Pipeline) NewNode(at *Node, after bool, nodeType int, symbol byte) *Node {
	node := pipeline.nodes[freeFirst].Next
	node.unlink()
	pipeline.Free--
	place := at
	if after {
		place = at.Next
	}
	node.linkBefore(place)
	node.SetSymbol(symbol)
	node.SetValue(0)
	node.SetSecond(0)
	node.Data[0x19-8] = 0
	node.Data[0x14-8] = 0
	node.SetFlags(node.Flags()&^lowFlags | uint32(nodeType)&typeBits)
	if cursor := pipeline.Current; cursor != nil {
		if at == cursor.End && after {
			cursor.End = node
			return node
		}
		if at == cursor.Start && !after {
			cursor.Start = node
		}
	}
	return node
}

// FreeNode unlinks the node, puts it at the end of the free list and returns its next node, or its previous one
// when not forward. It returns nil instead when the node was the current cursor's last node going forward, or
// its first going back, and the cursor's window shrinks by the node (pipeline.md §2.3).
func (pipeline *Pipeline) FreeNode(node *Node, forward bool) *Node {
	neighbour := node.Previous
	if forward {
		neighbour = node.Next
	}
	if cursor := pipeline.Current; cursor != nil {
		if node == cursor.End {
			if cursor.End == cursor.Start {
				cursor.End, cursor.Start = nil, nil
			} else {
				cursor.End = cursor.End.Previous
			}
			if forward {
				neighbour = nil
			}
		}
		if node == cursor.Start {
			cursor.Start = node.Next
			if !forward {
				neighbour = nil
			}
		}
	}
	node.unlink()
	node.linkBefore(&pipeline.nodes[freeLast])
	pipeline.Free++
	node.Data[0] = node.Data[0]&^1 | Free
	return neighbour
}

// Next and Previous step through the current cursor's window: nil past its last node or before its first.
func (pipeline *Pipeline) Next(node *Node) *Node {
	if node == pipeline.Current.End {
		return nil
	}
	return node.Next
}

func (pipeline *Pipeline) Previous(node *Node) *Node {
	if node == pipeline.Current.Start {
		return nil
	}
	return node.Previous
}
