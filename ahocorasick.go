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
	NODES_WIDTH = 1024
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

	child []*node // A non-nil entry in this array means that the
	// index represents a byte value which can be
	// appended to the current node. Blices in the
	// trie are built up byte by byte through these
	// child node pointers.

	fails []*node // Where to fail to (by following the fail
	// pointers) for each possible byte

	suffix *node // Pointer to the longest possible strict suffix of
	// this node

	fail *node // Pointer to the next node which is in the dictionary
	// which can be reached from here following suffixes. Called fail
	// because it is used to fallback in the trie when a match fails.
}

// Matcher is returned by NewMatcher and contains a list of blices to
// match against
type Matcher struct {
	counter int // Counts the number of matches done, and is used to
	// prevent output of multiple matches of the same string
	root   *node // Points to trie[0]
}

func (n *node) getChild(i int) *node {
	if n.child == nil {
		return nil
	}
	return n.child[i]
}

func (n *node) setChild(i int, v *node) {
	if n.child == nil {
		n.child = make([]*node, 256)
	}
	n.child[i] = v
}

func (n *node) getFails(i int) *node {
	if n.fails == nil {
		return nil
	}
	return n.fails[i]
}

func (n *node) setFails(i int, v *node) {
	if n.fails == nil {
		n.fails = make([]*node, 256)
	}
	n.fails[i] = v
}

// finndBlice looks for a blice in the trie starting from the root and
// returns a pointer to the node representing the end of the blice. If
// the blice is not found it returns nil.
func (m *Matcher) findBlice(b []byte) *node {
	n := m.root

	for n != nil && len(b) > 0 {
		n = n.getChild(int(b[0]))
		b = b[1:]
	}

	return n
}

// buildTrie builds the fundamental trie structure from a set of
// blices.
func (m *Matcher) buildTrie(dictionary [][]byte) {

	max := 1
	for _, blice := range dictionary {
		max += len(blice)
	}

	nodes := make([][]*node, max / NODES_WIDTH + 1)
	for i := 0; i < max / NODES_WIDTH + 1; i++ {
		nodes[i] = make([]*node, NODES_WIDTH)
	}

	m.root = &node{root: true}
	nodes[0][0] = m.root
	nodes_counter := 1


	// This loop builds the nodes in the trie by following through
	// each dictionary entry building the children pointers.

	for i, blice := range dictionary {
		n := m.root
		var path []byte
		for _, b := range blice {
			path = append(path, b)

			c := n.getChild(int(b))

			if c == nil {
				c = &node{}
				nodes[nodes_counter / NODES_WIDTH][nodes_counter % NODES_WIDTH] = c
				nodes_counter += 1

				n.setChild(int(b), c)
				c.b = make([]byte, len(path))
				copy(c.b, path)

				// Nodes directly under the root node will have the
				// root as their fail point as there are no suffixes
				// possible.

				if len(path) == 1 {
					c.fail = m.root
				}

				c.suffix = m.root
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
			c := n.getChild(i)
			if c != nil {
				l.PushBack(c)

				for j := 1; j < len(c.b); j++ {
					c.fail = m.findBlice(c.b[j:])
					if c.fail != nil {
						break
					}
				}

				if c.fail == nil {
					c.fail = m.root
				}

				for j := 1; j < len(c.b); j++ {
					s := m.findBlice(c.b[j:])
					if s != nil && s.output {
						c.suffix = s
						break
					}
				}
			}
		}
	}

	for i := 0; i < nodes_counter; i++ {
		for c := 0; c < 256; c++ {
			n := nodes[i / NODES_WIDTH][i % NODES_WIDTH]
			for n.getChild(c) == nil && !n.root {
				n = n.fail
			}
			nodes[i / NODES_WIDTH][i % NODES_WIDTH].setFails(c, n)
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

		if !n.root && n.getChild(c) == nil {
			n = n.getFails(c)
		}

		f := n.getChild(c)
		if f != nil {
			n = f

			if f.output && f.counter != m.counter {
				hits = append(hits, f.index)
				f.counter = m.counter
			}

			for !f.suffix.root {
				f = f.suffix
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
