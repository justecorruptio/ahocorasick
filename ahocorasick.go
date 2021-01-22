// ahocorasick.go: implementation of the Aho-Corasick string matching
// algorithm. Actually implemented as matching against []byte rather
// than the Go string type. Throughout this code []byte is referred to
// as a blice.
//
// http://en.wikipedia.org/wiki/Aho%E2%80%93Corasick_string_matching_algorithm
//
// Copyright (c) 2013 CloudFlare, Inc.

package ahocorasick

import (
	"container/list"
)

const (
	TABLE_WIDTH = 1024
)

// A node in the trie structure used to implement Aho-Corasick
type node struct {
	b []byte // The blice at this node

	output bool // True means this node represents a blice that should
	// be output when matching
	index int // index into original dictionary if output is true

	// The use of fixed size arrays is space-inefficient but fast for
	// lookups.

	child []int32 // A non-nil entry in this array means that the
	// index represents a byte value which can be
	// appended to the current node. Blices in the
	// trie are built up byte by byte through these
	// child node pointers.

	suffix int32 // Pointer to the longest possible strict suffix of
	// this node

	fail int32 // Pointer to the next node which is in the dictionary
	// which can be reached from here following suffixes. Called fail
	// because it is used to fallback in the trie when a match fails.

}

// Matcher is returned by NewMatcher and contains a list of blices to
// match against
type Matcher struct {
	root   *node // Points to trie[0]

	table [][]*node
	tableSize int32
}

func (m *Matcher) tableGet(i int32) *node {
	return m.table[i / TABLE_WIDTH][i % TABLE_WIDTH]
}

func (m *Matcher) tableSet(i int32, n *node) {
	if i / TABLE_WIDTH >= int32(len(m.table)) {
		m.table = append(m.table, make([]*node, TABLE_WIDTH))
	}
	m.table[i / TABLE_WIDTH][i % TABLE_WIDTH] = n
}

func (n *node) getChild(i byte) int32 {
	if n.child == nil {
		return 0
	}
	return n.child[i]
}

func (n *node) setChild(i byte, v int32) {
	if n.child == nil {
		n.child = make([]int32, 256)
	}
	n.child[i] = v
}

// finndBlice looks for a blice in the trie starting from the root and
// returns a pointer to the node representing the end of the blice. If
// the blice is not found it returns nil.
func (m *Matcher) findBlice(b []byte) int32 {
	i := int32(1)

	for i != 0 && len(b) > 0 {
		i = m.tableGet(i).getChild(b[0])
		b = b[1:]
	}

	return i
}

// buildTrie builds the fundamental trie structure from a set of
// blices.
func (m *Matcher) buildTrie(dictionary [][]byte) {

	m.table = [][]*node{}

	m.root = &node{}
	m.tableSet(0, nil)
	m.tableSet(1, m.root)
	m.tableSize = 2

	// This loop builds the nodes in the trie by following through
	// each dictionary entry building the children pointers.

	for i, blice := range dictionary {
		n := m.root
		var path []byte
		for _, b := range blice {
			path = append(path, b)

			c := m.tableGet(n.getChild(b))

			if c == nil {
				c = &node{}
				m.tableSet(m.tableSize, c)

				n.setChild(b, m.tableSize)
				m.tableSize += 1

				c.b = make([]byte, len(path))
				copy(c.b, path)

				// Nodes directly under the root node will have the
				// root as their fail point as there are no suffixes
				// possible.

				if len(path) == 1 {
					c.fail = 1
				}

				c.suffix = 1
			}

			n = c
		}

		// The last value of n points to the node representing a
		// dictionary entry

		n.output = true
		n.index = i
	}

	l := new(list.List)
	l.PushBack(m.root)

	for l.Len() > 0 {
		n := l.Remove(l.Front()).(*node)

		for i := 0; i <= 255; i++ {
			c := m.tableGet(n.getChild(byte(i)))
			if c != nil {
				l.PushBack(c)

				for j := 1; j < len(c.b); j++ {
					c.fail = m.findBlice(c.b[j:])
					if c.fail != 0 {
						break
					}
				}

				if c.fail == 0 {
					c.fail = 1
				}

				for j := 1; j < len(c.b); j++ {
					si := m.findBlice(c.b[j:])
					if si != 0 && m.tableGet(si).output {
						c.suffix = si
						break
					}
				}
			}
		}
	}
}

// NewMatcher creates a new Matcher used to match against a set of
// blices
func NewMatcher(dictionary [][]byte) *Matcher {
	m := new(Matcher)

	m.buildTrie(dictionary)

	return m
}

// NewStringMatcher creates a new Matcher used to match against a set
// of strings (this is a helper to make initialization easy)
func NewStringMatcher(dictionary []string) *Matcher {
	m := new(Matcher)

	var d [][]byte
	for _, s := range dictionary {
		d = append(d, []byte(s))
	}

	m.buildTrie(d)

	return m
}

// Match searches in for blices and returns all the blices found as
// indexes into the original dictionary
func (m *Matcher) Match(in []byte) []int {
	marks := make(map[int32]bool)

	var hits []int

	ni := int32(1)
	n := m.root

	for _, b := range in {
		if ni != 1 && n.getChild(b) == 0 {
			ni = n.fail
			n = m.tableGet(n.fail)
		}

		fi := n.getChild(b)
		if fi != 0 {
			f := m.tableGet(fi)
			ni = fi
			n = f

			_, marked := marks[fi]
			if f.output && !marked {
				hits = append(hits, f.index)
				marks[fi] = true
			}

			for f.suffix != 1 {
				fi = f.suffix
				f = m.tableGet(fi)
				_, marked := marks[fi]
				if !marked {
					hits = append(hits, f.index)
					marks[fi] = true
				} else {

					// There's no point working our way up the
					// suffixes if it's been done before for this call
					// to Match. The matches are already in hits.

					break
				}
			}
		}
	}

	return hits
}
