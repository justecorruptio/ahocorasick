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
	root bool // true if this is the root

	b []byte // The blice at this node

	output bool // True means this node represents a blice that should
	// be output when matching
	index int // index into original dictionary if output is true

	counter int // Set to the value of the Matcher.counter when a
	// match is output to prevent duplicate output

	// The use of fixed size arrays is space-inefficient but fast for
	// lookups.

	child []int32 // A non-nil entry in this array means that the
	// index represents a byte value which can be
	// appended to the current node. Blices in the
	// trie are built up byte by byte through these
	// child node pointers.

	fails []int32 // Where to fail to (by following the fail
	// pointers) for each possible byte

	suffix int32 // Pointer to the longest possible strict suffix of
	// this node

	fail int32 // Pointer to the next node which is in the dictionary
	// which can be reached from here following suffixes. Called fail
	// because it is used to fallback in the trie when a match fails.

}

// Matcher is returned by NewMatcher and contains a list of blices to
// match against
type Matcher struct {
	counter int // Counts the number of matches done, and is used to
	// prevent output of multiple matches of the same string
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

func (n *node) getChild(i int) int32 {
	if n.child == nil {
		return 0
	}
	return n.child[i]
}

func (n *node) setChild(i int, v int32) {
	if n.child == nil {
		n.child = make([]int32, 256)
	}
	n.child[i] = v
}

func (n *node) getFails(i int) int32 {
	if n.fails == nil {
		return 0
	}
	return n.fails[i]
}

func (n *node) setFails(i int, v int32) {
	if n.fails == nil {
		n.fails = make([]int32, 256)
	}
	n.fails[i] = v
}

// finndBlice looks for a blice in the trie starting from the root and
// returns a pointer to the node representing the end of the blice. If
// the blice is not found it returns nil.
func (m *Matcher) findBlice(b []byte) int32 {
	n := m.root
	i := int32(1)

	for n != nil && len(b) > 0 {
		i = n.getChild(int(b[0]))
		n = m.tableGet(i)
		b = b[1:]
	}

	return i
}

// buildTrie builds the fundamental trie structure from a set of
// blices.
func (m *Matcher) buildTrie(dictionary [][]byte) {

	m.table = [][]*node{}

	m.root = &node{root: true}
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

			c := m.tableGet(n.getChild(int(b)))

			if c == nil {
				c = &node{}
				m.tableSet(m.tableSize, c)

				n.setChild(int(b), m.tableSize)
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

		for i := 0; i < 256; i++ {
			c := m.tableGet(n.getChild(i))
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

	for i := int32(1); i < m.tableSize; i++ {
		for c := 0; c < 256; c++ {
			n := m.tableGet(i)
			j := i
			for n.getChild(c) == 0 && !n.root {
				j = n.fail
				n = m.tableGet(j)
			}
			m.tableGet(i).setFails(c, j)
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
	m.counter += 1
	var hits []int

	n := m.root

	for _, b := range in {
		c := int(b)

		if !n.root && m.tableGet(n.getChild(c)) == nil {
			n = m.tableGet(n.getFails(c))
		}

		f := m.tableGet(n.getChild(c))
		if f != nil {
			n = f

			if f.output && f.counter != m.counter {
				hits = append(hits, f.index)
				f.counter = m.counter
			}

			for !m.tableGet(f.suffix).root {
				f = m.tableGet(f.suffix)
				if f.counter != m.counter {
					hits = append(hits, f.index)
					f.counter = m.counter
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
