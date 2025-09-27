package cmd

import "github.com/pmenglund/stacky/internal/stackgraph"

// cloneBranchTree returns a deep copy of the branch and its descendants.
func cloneBranchTree(src *stackgraph.Branch) *stackgraph.Branch {
	if src == nil {
		return nil
	}
	clone := cloneBranchShallow(src)
	if len(src.Children) == 0 {
		return clone
	}
	clone.Children = make([]*stackgraph.Branch, len(src.Children))
	for i, child := range src.Children {
		childClone := cloneBranchTree(child)
		if childClone != nil {
			childClone.Parent = clone
		}
		clone.Children[i] = childClone
	}
	return clone
}

// cloneBranchShallow copies identifying metadata from src without descendants.
func cloneBranchShallow(src *stackgraph.Branch) *stackgraph.Branch {
	if src == nil {
		return nil
	}
	return &stackgraph.Branch{
		Name:         src.Name,
		ParentCommit: src.ParentCommit,
		Commit:       src.Commit,
	}
}

func stackForestForBranch(current *stackgraph.Branch) []*stackgraph.Branch {
	if current == nil {
		return nil
	}

	subtree := cloneBranchTree(current)
	for parent := current.Parent; parent != nil; parent = parent.Parent {
		parentClone := cloneBranchShallow(parent)
		subtree.Parent = parentClone
		parentClone.Children = []*stackgraph.Branch{subtree}
		subtree = parentClone
	}
	return []*stackgraph.Branch{subtree}
}

func upstackForestForBranch(current *stackgraph.Branch) []*stackgraph.Branch {
	if current == nil {
		return nil
	}
	return []*stackgraph.Branch{cloneBranchTree(current)}
}

func downstackForestForBranch(current *stackgraph.Branch) []*stackgraph.Branch {
	if current == nil {
		return nil
	}

	node := cloneBranchShallow(current)
	for parent := current.Parent; parent != nil; parent = parent.Parent {
		parentClone := cloneBranchShallow(parent)
		node.Parent = parentClone
		parentClone.Children = []*stackgraph.Branch{node}
		node = parentClone
	}
	return []*stackgraph.Branch{node}
}

func forestBranchNames(forest []*stackgraph.Branch) map[string]struct{} {
	names := make(map[string]struct{})
	for _, name := range forestBranchList(forest) {
		names[name] = struct{}{}
	}
	return names
}

func forestBranchList(forest []*stackgraph.Branch) []string {
	if len(forest) == 0 {
		return nil
	}
	var result []string
	var walk func(*stackgraph.Branch)
	walk = func(b *stackgraph.Branch) {
		if b == nil {
			return
		}
		result = append(result, b.Name)
		for _, child := range b.Children {
			walk(child)
		}
	}
	for _, root := range forest {
		walk(root)
	}
	return result
}

func filterOutBranch(names []string, exclude string) []string {
	if exclude == "" {
		return names
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == exclude {
			continue
		}
		out = append(out, name)
	}
	return out
}
