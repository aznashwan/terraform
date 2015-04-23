package terraform

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform/dag"
)

// GraphNodeDotter can be implemented by a node to cause it to be included
// in the dot graph. The Dot method will be called which is expected to
// return a representation of this node.
type GraphNodeDotter interface {
	// Dot is called to return the dot formatting for the node.
	// The first parameter is the title of the node.
	// The second parameter includes user-specified options that affect the dot
	// graph. See GraphDotOpts below for details.
	Dot(string, *GraphDotContext) string
}

type GraphNodeDotterRanked interface {
	DotRank() int
}

// GraphDotOpts are the options for generating a dot formatted Graph.
type GraphDotOpts struct {
	// Allows some nodes to decide to only show themselves when the user has
	// requested the "verbose" graph.
	Verbose bool

	// Highlight Cycles
	DrawCycles bool
}

type GraphDotContext struct {
	Opts        *GraphDotOpts
	Cycles      [][]dag.Vertex
	CurrentRank int
}

type drawableVertex struct {
	Vertex dag.Vertex
	Rank   int
}

// GraphDot returns the dot formatting of a visual representation of
// the given Terraform graph.
func GraphDot(g *Graph, opts *GraphDotOpts) (string, error) {
	buf := new(bytes.Buffer)

	// Start the graph
	buf.WriteString("digraph {\n")
	buf.WriteString("\tcompound = true;\n")

	// Find and rank drawable vertices by doing a depth first walk from the nodes
	// that rank themselves 0
	drawableVertices := make(map[dag.Vertex]int)
	rankedVertices := make(map[int][]dag.Vertex)

	var startFrom []dag.Vertex
	for _, v := range g.Vertices() {
		if dr, ok := v.(GraphNodeDotterRanked); ok {
			if dr.DotRank() == 0 {
				startFrom = append(startFrom, v)
			}
		}
	}

	ctx := &GraphDotContext{
		Opts:   opts,
		Cycles: g.Cycles(),
	}

	walk := func(v dag.Vertex, depth int) error {
		// We only care about nodes that yield non-empty Dot strings.
		if dn, ok := v.(GraphNodeDotter); !ok {
			return nil
		} else if dn.Dot("fake", ctx) == "" {
			return nil
		}

		// Allow a node to override its rank in the Dot
		if dr, ok := v.(GraphNodeDotterRanked); ok {
			depth = dr.DotRank()
		}

		// Otherwise the graph depth is the rank
		drawableVertices[v] = depth
		rankedVertices[depth] = append(rankedVertices[depth], v)
		return nil
	}

	if err := g.ReverseDepthFirstWalk(startFrom, walk); err != nil {
		return "", err
	}

	// Now we draw each rank
	rank := 0
	vs := rankedVertices[rank]
	for len(vs) > 0 {
		// Begin rank block
		buf.WriteString(fmt.Sprintf("\tsubgraph rank%d {\n", rank))
		if rank == 0 {
			buf.WriteString("\t\trank = sink;\n")
		} else {
			buf.WriteString("\t\trank = same;\n")
		}

		// Sort by VertexName so the graph is consistent
		sort.Sort(dag.ByVertexName(vs))

		// Draw vertices
		for _, v := range vs {
			dn := v.(GraphNodeDotter)
			scanner := bufio.NewScanner(strings.NewReader(
				dn.Dot(dag.VertexName(v), ctx)))
			for scanner.Scan() {
				buf.WriteString("\t\t" + scanner.Text() + "\n")
			}
		}

		// Close rank block; edges must come outside of it
		buf.WriteString("\t}\n")

		for _, v := range vs {
			// Draw all the edges from this vertex to other nodes
			targets := dag.AsVertexList(g.DownEdges(v))
			sort.Sort(dag.ByVertexName(targets))
			for _, t := range targets {
				target := t.(dag.Vertex)
				if _, ok := drawableVertices[target]; !ok {
					continue
				}

				buf.WriteString(fmt.Sprintf(
					"\t\"%s\" -> \"%s\";\n",
					dag.VertexName(v),
					dag.VertexName(target)))
			}
		}
		rank++
		vs = rankedVertices[rank]
	}

	if opts.DrawCycles {
		colors := []string{"red", "green", "blue"}
		for ci, cycle := range ctx.Cycles {
			cycleEdges := make([]string, 0, len(cycle))
			for i, c := range cycle {
				// Catch the last wrapping edge of the cycle
				if i+1 >= len(cycle) {
					i = -1
				}
				cycleEdges = append(cycleEdges, fmt.Sprintf(
					"\t\"%s\" -> \"%s\" [color=%s, penwidth=2.0];\n",
					dag.VertexName(c),
					dag.VertexName(cycle[i+1]),
					colors[ci%len(colors)]))
			}

			// Sort to get consistent graph output
			sort.Strings(cycleEdges)

			for _, edge := range cycleEdges {
				buf.WriteString(edge)
			}
		}
	}

	// End the graph
	buf.WriteString("}\n")
	return buf.String(), nil
}
