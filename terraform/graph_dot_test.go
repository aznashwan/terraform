package terraform

import (
	"strings"
	"testing"
)

func TestGraphDot(t *testing.T) {
	cases := map[string]struct {
		Graph  testGraphFunc
		Opts   GraphDotOpts
		Expect string
		Error  bool
	}{
		"empty": {
			Graph: func() *Graph { return &Graph{} },
			Expect: `
digraph {
	compound = true;
}
			`,
		},
		"three-level": {
			Graph: func() *Graph {
				var g Graph
				root := &testDrawableRanked{VertexName: "root", Rank: 0}
				g.Add(root)

				levelOne := []string{"foo", "bar"}
				for _, s := range levelOne {
					g.Add(&testDrawable{
						VertexName:      s,
						DependentOnMock: []string{"root"},
					})
				}

				levelTwo := []string{"baz", "qux"}
				for i, s := range levelTwo {
					g.Add(&testDrawable{
						VertexName:      s,
						DependentOnMock: levelOne[i : i+1],
					})
				}

				g.ConnectDependents()
				return &g
			},
			Expect: `
digraph {
	compound = true;
	subgraph rank0 {
		rank = sink;
		root
	}
	subgraph rank1 {
		rank = same;
		bar
		foo
	}
	"bar" -> "root";
	"foo" -> "root";
	subgraph rank2 {
		rank = same;
		baz
		qux
	}
	"baz" -> "foo";
	"qux" -> "bar";
}
			`,
		},
		"cycle": {
			Opts: GraphDotOpts{
				DrawCycles: true,
			},
			Graph: func() *Graph {
				var g Graph
				root := &testDrawableRanked{VertexName: "root", Rank: 0}
				g.Add(root)

				g.Add(&testDrawable{
					VertexName:      "A",
					DependentOnMock: []string{"root", "C"},
				})

				g.Add(&testDrawable{
					VertexName:      "B",
					DependentOnMock: []string{"A"},
				})

				g.Add(&testDrawable{
					VertexName:      "C",
					DependentOnMock: []string{"B"},
				})

				g.ConnectDependents()
				return &g
			},
			Expect: `
digraph {
	compound = true;
	subgraph rank0 {
		rank = sink;
		root
	}
	subgraph rank1 {
		rank = same;
		A
	}
	"A" -> "C";
	"A" -> "root";
	subgraph rank2 {
		rank = same;
		B
	}
	"B" -> "A";
	subgraph rank3 {
		rank = same;
		C
	}
	"C" -> "B";
	"A" -> "B" [color=red, penwidth=2.0];
	"B" -> "C" [color=red, penwidth=2.0];
	"C" -> "A" [color=red, penwidth=2.0];
}
			`,
		},
	}

	for tn, tc := range cases {
		actual, err := GraphDot(tc.Graph(), &tc.Opts)
		if (err != nil) != tc.Error {
			t.Fatalf("%s: expected err: %t, got: %s", tn, tc.Error, err)
		}

		expected := strings.TrimSpace(tc.Expect) + "\n"
		if actual != expected {
			t.Fatalf("%s:\n\nexpected:\n%s\n\ngot:\n%s", tn, expected, actual)
		}
	}
}

type testGraphFunc func() *Graph

type testDrawable struct {
	VertexName      string
	DependentOnMock []string
}

func (node *testDrawable) Name() string {
	return node.VertexName
}
func (node *testDrawable) Dot(n string, ctx *GraphDotContext) string {
	return node.VertexName
}
func (node *testDrawable) DependableName() []string {
	return []string{node.VertexName}
}
func (node *testDrawable) DependentOn() []string {
	return node.DependentOnMock
}

type testDrawableRanked struct {
	VertexName      string
	Rank            int
	DependentOnMock []string
}

func (node *testDrawableRanked) Name() string {
	return node.VertexName
}
func (node *testDrawableRanked) Dot(n string, ctx *GraphDotContext) string {
	return node.VertexName
}
func (node *testDrawableRanked) DotRank() int {
	return node.Rank
}
func (node *testDrawableRanked) DependableName() []string {
	return []string{node.VertexName}
}
func (node *testDrawableRanked) DependentOn() []string {
	return node.DependentOnMock
}
