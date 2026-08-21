// Package cli implements tick's scriptable subcommands. Every mutation goes
// through store.Update so it stays consistent with the TUI.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/simontheguitarist/tick/internal/store"
)

// CurrentProject resolves the project for the working directory.
func CurrentProject() (path, name string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root := store.ProjectRoot(cwd)
	return root, filepath.Base(root), nil
}

// ResolveProject strips an optional "-p <path>" / "--project <path>" flag from
// args and returns the remaining args plus the resolved project path and name.
// Without the flag it uses the current working directory's project.
func ResolveProject(args []string) (rest []string, path, name string, err error) {
	var pflag string
	out := args[:0:0]
	for i := 0; i < len(args); i++ {
		if args[i] == "-p" || args[i] == "--project" {
			if i+1 >= len(args) {
				return nil, "", "", fmt.Errorf("%s needs a path", args[i])
			}
			pflag = args[i+1]
			i++
			continue
		}
		out = append(out, args[i])
	}
	if pflag != "" {
		root := store.ProjectRoot(pflag)
		return out, root, filepath.Base(root), nil
	}
	p, n, err := CurrentProject()
	return out, p, n, err
}

// Add appends a step to the project, creating it on first use.
func Add(projPath, projName, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("nothing to add — give me some step text")
	}
	if _, err := store.Update(func(s *store.Store) error {
		s.Ensure(projPath, projName).AddStep(text)
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("added to %s: %s\n", projName, text)
	return nil
}

// List prints the project's steps. Done steps are hidden unless all is set.
func List(projPath, projName string, all bool) error {
	s, err := store.Load()
	if err != nil {
		return err
	}
	p := s.Projects[projPath]
	if p == nil || len(p.Steps) == 0 {
		fmt.Printf("%s — no steps yet. Add one:  tick add \"...\"\n", projName)
		return nil
	}
	fmt.Println(projName)
	shown := 0
	for i, st := range p.Steps {
		if st.Done && !all {
			continue
		}
		shown++
		if st.Done {
			fmt.Printf("  %2d  \033[9;2m%s\033[0m \033[32m✓\033[0m\n", i+1, st.Text)
		} else if st.Important {
			fmt.Printf("  %2d  \033[1;31m!\033[0m %s\n", i+1, st.Text)
		} else {
			fmt.Printf("  %2d  %s\n", i+1, st.Text)
		}
	}
	if shown == 0 {
		fmt.Println("  (all done — nice. use --all to see them)")
	}
	return nil
}

// Done marks one or more steps done (1-based indices).
func Done(projPath, projName string, nums []int) error {
	var msgs []string
	if _, err := store.Update(func(s *store.Store) error {
		p, err := requireProject(s, projPath, projName)
		if err != nil {
			return err
		}
		if err := validate(p, nums); err != nil {
			return err
		}
		for _, n := range nums {
			st := &p.Steps[n-1]
			if !st.Done {
				now := store.Now()
				st.Done, st.DoneAt = true, &now
				store.Touch(st)
			}
			msgs = append(msgs, fmt.Sprintf("✓ %s", st.Text))
		}
		return nil
	}); err != nil {
		return err
	}
	printAll(msgs)
	return nil
}

// Undone flips steps back to open.
func Undone(projPath, projName string, nums []int) error {
	var msgs []string
	if _, err := store.Update(func(s *store.Store) error {
		p, err := requireProject(s, projPath, projName)
		if err != nil {
			return err
		}
		if err := validate(p, nums); err != nil {
			return err
		}
		for _, n := range nums {
			st := &p.Steps[n-1]
			st.Done, st.DoneAt = false, nil
			store.Touch(st)
			msgs = append(msgs, fmt.Sprintf("○ %s", st.Text))
		}
		return nil
	}); err != nil {
		return err
	}
	printAll(msgs)
	return nil
}

// Remove deletes steps outright (1-based indices).
func Remove(projPath, projName string, nums []int) error {
	var msgs []string
	if _, err := store.Update(func(s *store.Store) error {
		p, err := requireProject(s, projPath, projName)
		if err != nil {
			return err
		}
		if err := validate(p, nums); err != nil {
			return err
		}
		// Delete from the highest index down so earlier indices stay valid.
		for _, n := range sortedDesc(nums) {
			msgs = append(msgs, fmt.Sprintf("removed: %s", p.Steps[n-1].Text))
			s.RemoveStep(p, n-1)
		}
		return nil
	}); err != nil {
		return err
	}
	printAll(msgs)
	return nil
}

// Edit replaces the text of step n.
func Edit(projPath, projName string, n int, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("nothing to set — give me the new text")
	}
	if _, err := store.Update(func(s *store.Store) error {
		p, err := requireProject(s, projPath, projName)
		if err != nil {
			return err
		}
		if err := validate(p, []int{n}); err != nil {
			return err
		}
		p.Steps[n-1].Text = text
		store.Touch(&p.Steps[n-1])
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("edited %d: %s\n", n, text)
	return nil
}

// Projects lists all tracked projects with their open-step counts.
func Projects(currentPath string) error {
	s, err := store.Load()
	if err != nil {
		return err
	}
	if len(s.Projects) == 0 {
		fmt.Println("no projects tracked yet — run `tick add \"...\"` inside one")
		return nil
	}
	for _, p := range s.SortedProjects() {
		marker := " "
		if p.Path == currentPath {
			marker = "*" // the project you're standing in
		}
		loc := p.Path
		if loc == "" {
			loc = "(not linked — run `tick link " + p.Name + "` inside its directory)"
		}
		fmt.Printf(" %s %-24s %2d open   %s\n", marker, p.Name, p.OpenCount(), loc)
	}
	return nil
}

// Untrack removes a whole project (and its steps) from the store.
func Untrack(projPath, projName string, assumeYes bool) error {
	s, err := store.Load()
	if err != nil {
		return err
	}
	p := s.Projects[projPath]
	if p == nil {
		return fmt.Errorf("%s is not tracked", projName)
	}
	if !assumeYes {
		if !confirm(fmt.Sprintf("Untrack %q and delete its %d step(s)? [y/N] ", projName, len(p.Steps))) {
			fmt.Println("aborted")
			return nil
		}
	}
	if _, err := store.Update(func(s *store.Store) error {
		if p := s.Projects[projPath]; p != nil {
			s.Untrack(p)
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("untracked %s\n", projName)
	return nil
}

// Clear removes done steps from the current project, or from all projects.
func Clear(projPath, projName string, allProjects bool) error {
	removed := 0
	if _, err := store.Update(func(s *store.Store) error {
		targets := []*store.Project{}
		if allProjects {
			targets = s.SortedProjects()
		} else {
			p, err := requireProject(s, projPath, projName)
			if err != nil {
				return err
			}
			targets = append(targets, p)
		}
		for _, p := range targets {
			for i := len(p.Steps) - 1; i >= 0; i-- {
				if p.Steps[i].Done {
					s.RemoveStep(p, i)
					removed++
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("cleared %d done step(s)\n", removed)
	return nil
}

// --- helpers ---

func requireProject(s *store.Store, path, name string) (*store.Project, error) {
	p := s.Projects[path]
	if p == nil {
		return nil, fmt.Errorf("%s is not tracked yet — add a step first", name)
	}
	return p, nil
}

func validate(p *store.Project, nums []int) error {
	for _, n := range nums {
		if n < 1 || n > len(p.Steps) {
			return fmt.Errorf("no step %d (project has %d)", n, len(p.Steps))
		}
	}
	return nil
}

// ParseNums turns string args into 1-based step numbers.
func ParseNums(args []string) ([]int, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("need at least one step number")
	}
	out := make([]int, 0, len(args))
	for _, a := range args {
		n, err := strconv.Atoi(a)
		if err != nil {
			return nil, fmt.Errorf("not a step number: %q", a)
		}
		out = append(out, n)
	}
	return out, nil
}

func sortedDesc(nums []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(nums))
	for _, n := range nums {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func printAll(msgs []string) {
	for _, m := range msgs {
		fmt.Println(m)
	}
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(sc.Text())) {
	case "y", "yes":
		return true
	}
	return false
}
