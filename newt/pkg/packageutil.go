package pkg

import (
	"sort"
)

type lpkgSorter struct {
	pkgs []*LocalPackage
}

func (s lpkgSorter) Len() int {
	return len(s.pkgs)
}
func (s lpkgSorter) Swap(i, j int) {
	s.pkgs[i], s.pkgs[j] = s.pkgs[j], s.pkgs[i]
}
func (s lpkgSorter) Less(i, j int) bool {
	return s.pkgs[i].Name() < s.pkgs[j].Name()
}

func SortLclPkgs(pkgs []*LocalPackage) []*LocalPackage {
	sorter := lpkgSorter{
		pkgs: make([]*LocalPackage, 0, len(pkgs)),
	}

	for _, p := range pkgs {
		sorter.pkgs = append(sorter.pkgs, p)
	}

	sort.Sort(sorter)
	return sorter.pkgs
}
