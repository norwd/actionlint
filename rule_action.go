package actionlint

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MinimumNodeRunnerVersion is the minimum supported Node.js version for JavaScript action runner.
// This constant will be updated when GitHub bumps the version.
// https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs-for-javascript-actions
//
// Note: "node16" runner is deprecated but still available: https://github.blog/changelog/2023-09-22-github-actions-transitioning-from-node-16-to-node-20/
const MinimumNodeRunnerVersion uint64 = 16

// RuleAction is a rule to check running action in steps of jobs.
// https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idstepsuses
type RuleAction struct {
	RuleBase
	cache *LocalActionsCache
}

// NewRuleAction creates new RuleAction instance.
func NewRuleAction(cache *LocalActionsCache) *RuleAction {
	return &RuleAction{
		RuleBase: RuleBase{
			name: "action",
			desc: "Checks for popular actions released on GitHub, local actions, and action calls at \"uses:\"",
		},
		cache: cache,
	}
}

// VisitStep is callback when visiting Step node.
func (rule *RuleAction) VisitStep(n *Step) error {
	e, ok := n.Exec.(*ExecAction)
	if !ok || e.Uses == nil {
		return nil
	}

	if e.Uses.ContainsExpression() {
		// Cannot parse specification made with interpolation. Give up
		return nil
	}

	spec := e.Uses.Value

	if strings.HasPrefix(spec, "./") {
		// Relative to repository root
		rule.checkLocalAction(spec, e)
		return nil
	}

	if strings.HasPrefix(spec, "docker://") {
		rule.checkDockerAction(spec, e)
		return nil
	}

	rule.checkRepoAction(spec, e)
	return nil
}

// Parse {owner}/{repo}@{ref} or {owner}/{repo}/{path}@{ref}
func (rule *RuleAction) checkRepoAction(spec string, exec *ExecAction) {
	s := spec
	idx := strings.IndexRune(s, '@')
	if idx == -1 {
		rule.invalidActionFormat(exec.Uses.Pos, spec, "ref is missing")
		return
	}
	ref := s[idx+1:]
	s = s[:idx] // remove {ref}

	idx = strings.IndexRune(s, '/')
	if idx == -1 {
		rule.invalidActionFormat(exec.Uses.Pos, spec, "owner is missing")
		return
	}

	owner := s[:idx]
	s = s[idx+1:] // eat {owner}

	repo := s
	if idx := strings.IndexRune(s, '/'); idx >= 0 {
		repo = s[:idx]
	}

	if owner == "" || repo == "" || ref == "" {
		rule.invalidActionFormat(exec.Uses.Pos, spec, "owner and repo and ref should not be empty")
	}

	meta, ok := PopularActions[spec]
	if !ok {
		rule.Debug("This action is not found in popular actions data set: %s", spec)
		return
	}
	if meta.SkipInputs {
		rule.Debug("This action skips to check inputs: %s", spec)
		return
	}

	rule.checkAction(meta, exec, func(m *ActionMetadata) string {
		return strconv.Quote(spec)
	})
}

func (rule *RuleAction) invalidActionFormat(pos *Pos, spec string, why string) {
	rule.Errorf(pos, "specifying action %q in invalid format because %s. available formats are \"{owner}/{repo}@{ref}\" or \"{owner}/{repo}/{path}@{ref}\"", spec, why)
}

func (rule *RuleAction) invalidRunsName(pos *Pos, name, action, path string) {
	rule.Errorf(pos, "invalid runner name %q at runs.using in local action %q defined at %q. see https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs to know valid runner names", name, action, path)
}

func (rule *RuleAction) missingRunsProp(pos *Pos, prop, ty, action, path string) {
	rule.Errorf(pos, `%q is required in "runs" section because %q is a %s action. the action is defined at %q`, prop, action, ty, path)
}

func (rule *RuleAction) checkInvalidRunsProps(pos *Pos, r *ActionMetadataRuns, ty, action, path string, props []string) {
	for _, prop := range props {
		invalid := prop == "main" && r.Main != "" ||
			prop == "pre" && r.Pre != "" ||
			prop == "pre-if" && r.PreIf != "" ||
			prop == "post" && r.Post != "" ||
			prop == "post-if" && r.PostIf != "" ||
			prop == "steps" && len(r.Steps) > 0 ||
			prop == "image" && r.Image != "" ||
			prop == "pre-entrypoint" && r.PreEntrypoint != "" ||
			prop == "entrypoint" && r.Entrypoint != "" ||
			prop == "post-entrypoint" && r.PostEntrypoint != "" ||
			prop == "args" && r.Args != nil ||
			prop == "env" && r.Env != nil

		if invalid {
			rule.Errorf(pos, `%q is not allowed in "runs" section because %q is a %s action. the action is defined at %q`, prop, action, ty, path)
		}
	}
}

func (rule *RuleAction) checkRunsFileExists(file, dir, prop, name string, pos *Pos) {
	f := filepath.FromSlash(file)
	if f == "" {
		return
	}
	p := filepath.Join(dir, f)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		rule.Errorf(pos, `file %q does not exist in %q. it is specified at %q key in "runs" section in %q action`, f, dir, prop, name)
	}
}

// https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs-for-docker-container-actions
func (rule *RuleAction) checkLocalDockerActionRuns(r *ActionMetadataRuns, dir, name string, pos *Pos) {
	if r.Image == "" {
		rule.missingRunsProp(pos, "image", "Docker", name, dir)
	} else if !strings.HasPrefix(r.Image, "docker://") {
		rule.checkRunsFileExists(r.Image, dir, "image", name, pos)
		if filepath.Base(filepath.FromSlash(r.Image)) != "Dockerfile" {
			rule.Errorf(pos, `the local file %q referenced from "image" key must be named "Dockerfile" in %q action. the action is defined at %q`, r.Image, name, dir)
		}
	}
	rule.checkRunsFileExists(r.PreEntrypoint, dir, "pre-entrypoint", name, pos)
	rule.checkRunsFileExists(r.Entrypoint, dir, "entrypoint", name, pos)
	rule.checkRunsFileExists(r.PostEntrypoint, dir, "post-entrypoint", name, pos)
	rule.checkInvalidRunsProps(pos, r, "Docker", name, dir, []string{"main", "pre", "pre-if", "post", "post-if", "steps"})
}

// https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs-for-composite-actions
func (rule *RuleAction) checkLocalCompositeActionRuns(r *ActionMetadataRuns, dir, name string, pos *Pos) {
	if r.Steps == nil {
		rule.missingRunsProp(pos, "steps", "Composite", name, dir)
	}
	rule.checkInvalidRunsProps(pos, r, "Composite", name, dir, []string{"main", "pre", "pre-if", "post", "post-if", "image", "pre-entrypoint", "entrypoint", "post-entrypoint", "args", "env"})
}

// https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs-for-javascript-actions
func (rule *RuleAction) checkLocalJavaScriptActionRuns(r *ActionMetadataRuns, dir, name string, pos *Pos) {
	u := r.Using
	v, err := strconv.ParseUint(u[len("node"):], 10, 0)
	if err != nil {
		rule.invalidRunsName(pos, u, name, dir)
		return
	}
	if v < MinimumNodeRunnerVersion {
		rule.Errorf(
			pos,
			`%q runner at "runs.using" is unavailable since the Node.js version is too old (%d < %d) in local action %q defined at %q. see https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs-for-javascript-actions`,
			u,
			v,
			MinimumNodeRunnerVersion,
			name,
			dir,
		)
	}

	if r.Main == "" {
		rule.missingRunsProp(pos, "main", "JavaScript", name, dir)
	} else {
		rule.checkRunsFileExists(r.Main, dir, "main", name, pos)
	}

	rule.checkRunsFileExists(r.Pre, dir, "pre", name, pos)
	if r.Pre == "" && r.PreIf != "" {
		rule.Errorf(pos, `"pre" is required when "pre-if" is specified in "runs" section in %q action. the action is defined at %q`, name, dir)
	}

	rule.checkRunsFileExists(r.Post, dir, "post", name, pos)
	if r.Post == "" && r.PostIf != "" {
		rule.Errorf(pos, `"post" is required when "post-if" is specified in "runs" section in %q action. the action is defined at %q`, name, dir)
	}

	rule.checkInvalidRunsProps(pos, r, "JavaScript", name, dir, []string{"steps", "image", "pre-entrypoint", "entrypoint", "post-entrypoint", "args", "env"})
}

// https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs
func (rule *RuleAction) checkLocalActionRuns(meta *ActionMetadata, pos *Pos) {
	r := &meta.Runs
	if r.Using == "" {
		rule.Errorf(pos, `"runs.using" is missing in local action %q defined at %q`, meta.Name, meta.Dir())
		return
	}

	if r.Using == "docker" {
		rule.checkLocalDockerActionRuns(r, meta.Dir(), meta.Name, pos)
		return
	}

	if r.Using == "composite" {
		rule.checkLocalCompositeActionRuns(r, meta.Dir(), meta.Name, pos)
		return
	}

	if !strings.HasPrefix(r.Using, "node") {
		rule.invalidRunsName(pos, r.Using, meta.Name, meta.Dir())
		return
	}

	rule.checkLocalJavaScriptActionRuns(r, meta.Dir(), meta.Name, pos)
}

// https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#example-using-the-github-packages-container-registry
func (rule *RuleAction) checkDockerAction(uri string, exec *ExecAction) {
	tag := ""
	tagExists := false
	if idx := strings.IndexRune(uri[len("docker://"):], ':'); idx != -1 {
		idx += len("docker://")
		if idx < len(uri) {
			tag = uri[idx+1:]
			uri = uri[:idx]
			tagExists = true
		}
	}

	if _, err := url.Parse(uri); err != nil {
		rule.Errorf(
			exec.Uses.Pos,
			"URI for Docker container %q is invalid: %s (tag=%s)",
			uri,
			err.Error(),
			tag,
		)
	}

	if tagExists && tag == "" {
		rule.Errorf(exec.Uses.Pos, "tag of Docker action should not be empty: %q", uri)
	}
}

// https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#example-using-action-in-the-same-repository-as-the-workflow
func (rule *RuleAction) checkLocalAction(spec string, action *ExecAction) {
	meta, cached, err := rule.cache.FindMetadata(spec)
	if err != nil {
		rule.Error(action.Uses.Pos, err.Error())
		return
	}
	if meta == nil {
		return
	}

	if !cached {
		rule.Debug("Checking runner metadata of %s action %q at %q", meta.Runs, meta.Name, spec)
		rule.checkLocalActionRuns(meta, action.Uses.Pos)
	}

	rule.checkAction(meta, action, func(m *ActionMetadata) string {
		return fmt.Sprintf("%q defined at %q", m.Name, spec)
	})
}

func (rule *RuleAction) checkAction(meta *ActionMetadata, exec *ExecAction, describe func(*ActionMetadata) string) {
	// Check specified inputs are defined in action's inputs spec
	for id, i := range exec.Inputs {
		if _, ok := meta.Inputs[id]; !ok {
			ns := make([]string, 0, len(meta.Inputs))
			for _, i := range meta.Inputs {
				ns = append(ns, i.Name)
			}
			rule.Errorf(
				i.Name.Pos,
				"input %q is not defined in action %s. available inputs are %s",
				i.Name.Value,
				describe(meta),
				sortedQuotes(ns),
			)
		}
	}

	// Check mandatory inputs are specified
	for id, i := range meta.Inputs {
		if i.Required {
			if _, ok := exec.Inputs[id]; !ok {
				ns := make([]string, 0, len(meta.Inputs))
				for _, i := range meta.Inputs {
					if i.Required {
						ns = append(ns, i.Name)
					}
				}
				rule.Errorf(
					exec.Uses.Pos,
					"missing input %q which is required by action %s. all required inputs are %s",
					i.Name,
					describe(meta),
					sortedQuotes(ns),
				)
			}
		}
	}
}
