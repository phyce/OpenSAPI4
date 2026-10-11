package datapack

import "fmt"

type Question struct {
	Left  uint64
	Right uint64
}

type Node struct {
	Yes           int32
	No            int32
	Leaf          int32
	FirstQuestion uint32
	QuestionCount uint32
}

type Trees struct {
	SenoneCount   int
	Roots         []int32
	FirstSenones  []uint32
	SenoneCounts  []uint32
	Questions     []Question
	Nodes         []Node
	QuestionLists []uint16
}

const (
	WordInitial     = 1
	WordFinal       = 2
	SinglePhoneWord = 4
	InsideWord      = 8
)

func (trees *Trees) PhoneCount() int {
	return len(trees.Roots)
}

func (trees *Trees) Lookup(phone int, left int, right int, position int) int {
	node := trees.Nodes[trees.Roots[phone]]
	leftBit := uint64(1) << (left + 4)
	rightBit := uint64(1) << (right + 4)
	for node.Yes != -1 {
		next := node.No
		for _, number := range trees.QuestionLists[node.FirstQuestion : node.FirstQuestion+node.QuestionCount] {
			question := trees.Questions[number]
			if question.Left&uint64(position) != 0 && question.Left&leftBit != 0 && question.Right&rightBit != 0 {
				next = node.Yes
				break
			}
		}
		node = trees.Nodes[next]
	}
	return int(trees.FirstSenones[phone]) + int(node.Leaf)
}

func parseTrees(data []byte) (*Trees, error) {
	input, err := newReader(data, "WTRE")
	if err != nil {
		return nil, err
	}
	phoneCount := input.count(60)
	senoneCount := input.count(0x1FFF)
	questionCount := input.count(input.remaining(16))
	nodeCount := input.count(input.remaining(20))
	listLength := input.count(input.remaining(2))
	trees := &Trees{
		SenoneCount:  senoneCount,
		Roots:        input.int32s(phoneCount),
		FirstSenones: input.uint32s(phoneCount),
		SenoneCounts: input.uint32s(phoneCount),
	}
	masks := input.uint64s(2 * questionCount)
	trees.Questions = make([]Question, questionCount)
	for index := range trees.Questions {
		trees.Questions[index] = Question{Left: masks[2*index], Right: masks[2*index+1]}
	}
	fields := input.int32s(5 * nodeCount)
	trees.Nodes = make([]Node, nodeCount)
	for index := range trees.Nodes {
		record := fields[5*index : 5*index+5]
		trees.Nodes[index] = Node{Yes: record[0], No: record[1], Leaf: record[2], FirstQuestion: uint32(record[3]), QuestionCount: uint32(record[4])}
	}
	trees.QuestionLists = input.uint16s(listLength)
	if err := input.finish(); err != nil {
		return nil, err
	}
	if err := trees.validate(); err != nil {
		return nil, err
	}
	return trees, nil
}

func (trees *Trees) validate() error {
	for index, node := range trees.Nodes {
		if node.Yes == -1 {
			if node.No != -1 || node.Leaf < 0 {
				return fmt.Errorf("leaf node %d", index)
			}
			continue
		}
		if node.Yes <= int32(index) || node.No <= int32(index) || int(node.Yes) >= len(trees.Nodes) || int(node.No) >= len(trees.Nodes) {
			return fmt.Errorf("node %d links to %d and %d", index, node.Yes, node.No)
		}
		if node.QuestionCount == 0 || uint64(node.FirstQuestion)+uint64(node.QuestionCount) > uint64(len(trees.QuestionLists)) {
			return fmt.Errorf("node %d question list", index)
		}
		for _, question := range trees.QuestionLists[node.FirstQuestion : node.FirstQuestion+node.QuestionCount] {
			if int(question) >= len(trees.Questions) {
				return fmt.Errorf("node %d asks question %d", index, question)
			}
		}
	}
	for phone, root := range trees.Roots {
		first, count := trees.FirstSenones[phone], trees.SenoneCounts[phone]
		if count > 0 && (first == 0 || uint64(first)+uint64(count)-1 > uint64(trees.SenoneCount)) {
			return fmt.Errorf("phone %d senones %d..%d", phone, first, first+count-1)
		}
		if root == -1 {
			if count != 0 {
				return fmt.Errorf("phone %d has senones but no tree", phone)
			}
			continue
		}
		if root < 0 || int(root) >= len(trees.Nodes) {
			return fmt.Errorf("phone %d tree root %d", phone, root)
		}
		visited := make([]bool, len(trees.Nodes))
		pending := []int32{root}
		for len(pending) > 0 {
			index := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if visited[index] {
				continue
			}
			visited[index] = true
			node := trees.Nodes[index]
			if node.Yes == -1 {
				if uint32(node.Leaf) >= count {
					return fmt.Errorf("phone %d reaches leaf %d of %d", phone, node.Leaf, count)
				}
				continue
			}
			pending = append(pending, node.Yes, node.No)
		}
	}
	return nil
}
