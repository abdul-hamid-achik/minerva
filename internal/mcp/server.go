// Package mcp exposes Minerva's agent library and stack-readiness surface
// over the Model Context Protocol (stdio): skills, profiles, templates,
// presence/readiness probes, analytics, and suggestions.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/abdul-hamid-achik/minerva/internal/analytics"
	"github.com/abdul-hamid-achik/minerva/internal/bridge"
	"github.com/abdul-hamid-achik/minerva/internal/evidence"
	"github.com/abdul-hamid-achik/minerva/internal/integration"
	"github.com/abdul-hamid-achik/minerva/internal/learn"
	"github.com/abdul-hamid-achik/minerva/internal/library"
	"github.com/abdul-hamid-achik/minerva/internal/monitor"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/status"
	"github.com/abdul-hamid-achik/minerva/internal/suggest"
	"github.com/abdul-hamid-achik/minerva/internal/surface"
	"github.com/abdul-hamid-achik/minerva/internal/templates"
	"github.com/abdul-hamid-achik/minerva/internal/textdiff"
	"github.com/abdul-hamid-achik/minerva/internal/version"
)

const instructions = `Minerva is the agent library operator for ~/.agents and a stack readiness orchestrator.
It is NOT a second agent runtime.

Session-native surface (9 read-only tools): learn, status, suggest, resolve_skill,
skill, profile, library, stack_check, evidence.

- minerva_learn first if you do not know the contract.
- minerva_resolve_skill to pick a skill for the current task; then harness load_skill.
- Durable membership is minerva profile add-skills on the CLI (approval-gated).
- minerva skill activate does NOT inject into sonar/local-agent.

Do not reimplement MCPHub, Cortex, or Bob through Minerva.`

// Server wraps the go-sdk MCP server.
type Server struct {
	skillManager   *skill.Manager
	profileManager *profile.Manager
	analyticsStore *analytics.Store
	agentsDir      string
	srv            *sdkmcp.Server
}

// NewServer builds an MCP server with the given agents directory.
func NewServer(agentsDir string) (*Server, error) {
	if agentsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home: %w", err)
		}
		agentsDir = filepath.Join(home, ".agents")
	}

	skillMgr := skill.NewManagerWithState(agentsDir, filepath.Join(agentsDir, "skills"))
	if err := skillMgr.LoadAll(); err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}

	profileMgr := profile.NewManager(agentsDir)
	if err := profileMgr.LoadAll(); err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}

	analyticsStore := analytics.NewStore(agentsDir)
	_ = analyticsStore.Load() // best-effort

	s := &Server{
		skillManager:   skillMgr,
		profileManager: profileMgr,
		analyticsStore: analyticsStore,
		agentsDir:      agentsDir,
	}
	s.srv = sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "minerva", Version: version.Version},
		&sdkmcp.ServerOptions{Instructions: instructions},
	)
	s.register()
	return s, nil
}

// Run serves newline-delimited MCP JSON-RPC over stdio until cancellation.
func (s *Server) Run(ctx context.Context) error {
	return s.srv.Run(ctx, &sdkmcp.StdioTransport{})
}

func (s *Server) register() {
	for _, tool := range surface.ProductTools() {
		spec := readOnlyTool(tool.Name, tool.Title, tool.Description)
		switch tool.Name {
		case "minerva_learn":
			sdkmcp.AddTool(s.srv, spec, s.handleLearn)
		case "minerva_status":
			sdkmcp.AddTool(s.srv, spec, s.handleStatus)
		case "minerva_suggest":
			sdkmcp.AddTool(s.srv, spec, s.handleSuggest)
		case "minerva_resolve_skill":
			sdkmcp.AddTool(s.srv, spec, s.handleResolveSkill)
		case "minerva_skill":
			sdkmcp.AddTool(s.srv, spec, s.handleSkill)
		case "minerva_profile":
			sdkmcp.AddTool(s.srv, spec, s.handleProfile)
		case "minerva_library":
			sdkmcp.AddTool(s.srv, spec, s.handleLibrary)
		case "minerva_stack_check":
			sdkmcp.AddTool(s.srv, spec, s.handleStackCheck)
		case "minerva_evidence":
			sdkmcp.AddTool(s.srv, spec, s.handleEvidence)
		}
	}
}

func readOnlyTool(name, title, description string) *sdkmcp.Tool {
	destructive := false
	openWorld := false
	return &sdkmcp.Tool{
		Name: name, Title: title, Description: description,
		Annotations: &sdkmcp.ToolAnnotations{
			Title: title, ReadOnlyHint: true, DestructiveHint: &destructive,
			IdempotentHint: true, OpenWorldHint: &openWorld,
		},
	}
}

// --- Skill handlers ---

type SkillNameInput struct {
	Name string `json:"name" jsonschema:"required, skill name"`
}

type SkillCompareInput struct {
	NameA      string `json:"name_a" jsonschema:"required, first skill name"`
	NameB      string `json:"name_b" jsonschema:"required, second skill name"`
	SideBySide bool   `json:"side_by_side,omitempty" jsonschema:"if true, return full bodies instead of unified diff"`
}

type SkillCreateInput struct {
	Name        string `json:"name" jsonschema:"required, unique skill name"`
	Description string `json:"description,omitempty" jsonschema:"one-line description of what the skill does"`
	Content     string `json:"content" jsonschema:"required, markdown body of the skill"`
}

type SkillUpdateInput struct {
	Name        string  `json:"name" jsonschema:"required, skill name"`
	Description *string `json:"description,omitempty" jsonschema:"new one-line description; omit to keep current"`
	Content     *string `json:"content,omitempty" jsonschema:"new markdown body; omit to keep current"`
}

func (s *Server) handleSkillList(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	catalog := s.skillManager.Catalog()
	return textResult(catalog), catalog, nil
}

func (s *Server) handleSkillShow(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillNameInput) (*sdkmcp.CallToolResult, any, error) {
	content, ok := s.skillManager.Load(in.Name)
	if !ok {
		return errorResult(fmt.Sprintf("skill %q not found", in.Name)), nil, nil
	}
	return textResult(content), map[string]any{"name": in.Name, "content": content}, nil
}

func (s *Server) handleSkillCompare(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillCompareInput) (*sdkmcp.CallToolResult, any, error) {
	contentA, okA := s.skillManager.Load(in.NameA)
	contentB, okB := s.skillManager.Load(in.NameB)
	if !okA {
		return errorResult(fmt.Sprintf("skill %q not found", in.NameA)), nil, nil
	}
	if !okB {
		return errorResult(fmt.Sprintf("skill %q not found", in.NameB)), nil, nil
	}
	if in.SideBySide {
		result := map[string]any{
			"skill_a": map[string]any{"name": in.NameA, "content": contentA},
			"skill_b": map[string]any{"name": in.NameB, "content": contentB},
		}
		return textResult(result), result, nil
	}
	diff := textdiff.Unified(in.NameA, in.NameB, contentA, contentB)
	result := map[string]any{
		"identical": diff == "",
		"diff":      diff,
		"name_a":    in.NameA,
		"name_b":    in.NameB,
	}
	return textResult(result), result, nil
}

func (s *Server) handleSkillCreate(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillCreateInput) (*sdkmcp.CallToolResult, any, error) {
	skillsDir := filepath.Join(s.agentsDir, "skills")
	if err := s.skillManager.Create(skillsDir, in.Name, in.Description, in.Content); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("skill_create", in.Name, in.Description)
	return textResult(fmt.Sprintf("skill %q created", in.Name)), map[string]any{"created": in.Name}, nil
}

func (s *Server) handleSkillUpdate(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillUpdateInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.skillManager.Update(in.Name, in.Description, in.Content); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("skill_update", in.Name, "")
	return textResult(fmt.Sprintf("skill %q updated", in.Name)), map[string]any{"updated": in.Name}, nil
}

func (s *Server) handleSkillActivate(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillNameInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.skillManager.Activate(in.Name); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("skill_activate", in.Name, "")
	return textResult(fmt.Sprintf("skill %q activated", in.Name)), map[string]any{"activated": in.Name}, nil
}

func (s *Server) handleSkillDeactivate(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillNameInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.skillManager.Deactivate(in.Name); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("skill_deactivate", in.Name, "")
	return textResult(fmt.Sprintf("skill %q deactivated", in.Name)), map[string]any{"deactivated": in.Name}, nil
}

func (s *Server) handleSkillDelete(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillNameInput) (*sdkmcp.CallToolResult, any, error) {
	skillsDir := filepath.Join(s.agentsDir, "skills")
	if err := s.skillManager.Delete(skillsDir, in.Name); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(fmt.Sprintf("skill %q deleted", in.Name)), map[string]any{"deleted": in.Name}, nil
}

// --- Profile handlers ---

type ProfileNameInput struct {
	Name string `json:"name" jsonschema:"required, profile name"`
}

type ProfileCompareInput struct {
	NameA      string `json:"name_a" jsonschema:"required, first profile name"`
	NameB      string `json:"name_b" jsonschema:"required, second profile name"`
	SideBySide bool   `json:"side_by_side,omitempty" jsonschema:"if true, return full configs instead of unified diff"`
}

type ProfileCreateInput struct {
	Name         string   `json:"name" jsonschema:"required, unique profile name"`
	Description  string   `json:"description,omitempty" jsonschema:"one-line description"`
	Model        string   `json:"model,omitempty" jsonschema:"Ollama model to use"`
	Skills       []string `json:"skills,omitempty" jsonschema:"skill names to activate"`
	MCPServers   []string `json:"mcp_servers,omitempty" jsonschema:"MCP server names to allow"`
	SystemPrompt string   `json:"system_prompt,omitempty" jsonschema:"custom system prompt"`
}

type ProfileUpdatePromptInput struct {
	Name   string `json:"name" jsonschema:"required, profile name"`
	Prompt string `json:"prompt" jsonschema:"required, new system prompt"`
}

type ProfileUpdateSkillsInput struct {
	Name   string   `json:"name" jsonschema:"required, profile name"`
	Skills []string `json:"skills" jsonschema:"required, skill names"`
}

type ProfileUpdateModelInput struct {
	Name  string `json:"name" jsonschema:"required, profile name"`
	Model string `json:"model" jsonschema:"required, model id"`
}

type ProfileUpdateMCPInput struct {
	Name       string   `json:"name" jsonschema:"required, profile name"`
	MCPServers []string `json:"mcp_servers" jsonschema:"required, MCP server names"`
}

type ProfileUpdateDescInput struct {
	Name        string `json:"name" jsonschema:"required, profile name"`
	Description string `json:"description" jsonschema:"required, one-line description"`
}

func (s *Server) handleProfileList(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	payload := map[string]any{
		"profiles": s.profileManager.All(),
		"warnings": s.profileManager.Warnings(),
	}
	return textResult(payload), payload, nil
}

func (s *Server) handleProfileShow(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileNameInput) (*sdkmcp.CallToolResult, any, error) {
	p := s.profileManager.Get(in.Name)
	if p == nil {
		return errorResult(fmt.Sprintf("profile %q not found", in.Name)), nil, nil
	}
	return textResult(p), p, nil
}

func (s *Server) handleProfileCompare(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileCompareInput) (*sdkmcp.CallToolResult, any, error) {
	pA := s.profileManager.Get(in.NameA)
	pB := s.profileManager.Get(in.NameB)
	if pA == nil {
		return errorResult(fmt.Sprintf("profile %q not found", in.NameA)), nil, nil
	}
	if pB == nil {
		return errorResult(fmt.Sprintf("profile %q not found", in.NameB)), nil, nil
	}
	if in.SideBySide {
		result := map[string]any{"profile_a": pA, "profile_b": pB}
		return textResult(result), result, nil
	}
	ya := formatProfileProjection(pA)
	yb := formatProfileProjection(pB)
	diff := textdiff.Unified(in.NameA+"/agent.yaml", in.NameB+"/agent.yaml", ya, yb)
	result := map[string]any{
		"identical": diff == "",
		"diff":      diff,
		"name_a":    in.NameA,
		"name_b":    in.NameB,
	}
	return textResult(result), result, nil
}

func formatProfileProjection(p *profile.Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", p.Name)
	if p.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", p.Description)
	}
	if p.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", p.Model)
	}
	fmt.Fprintf(&b, "skills: [%s]\n", strings.Join(p.Skills, ", "))
	fmt.Fprintf(&b, "mcp_servers: [%s]\n", strings.Join(p.MCPServers, ", "))
	fmt.Fprintf(&b, "system_prompt: |\n")
	for _, line := range strings.Split(p.SystemPrompt, "\n") {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	return b.String()
}

func (s *Server) handleProfileCreate(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileCreateInput) (*sdkmcp.CallToolResult, any, error) {
	p := &profile.Profile{
		Name:         in.Name,
		Description:  in.Description,
		Model:        in.Model,
		Skills:       in.Skills,
		MCPServers:   in.MCPServers,
		SystemPrompt: in.SystemPrompt,
	}
	if err := s.profileManager.Create(p); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_create", in.Name, in.Description)
	return textResult(fmt.Sprintf("profile %q created", in.Name)), map[string]any{"created": in.Name}, nil
}

func (s *Server) handleProfileUpdatePrompt(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdatePromptInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.UpdateSystemPrompt(in.Name, in.Prompt); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_update_prompt", in.Name, "")
	return textResult(fmt.Sprintf("system prompt updated for profile %q", in.Name)), map[string]any{"updated": in.Name}, nil
}

func (s *Server) handleProfileUpdateSkills(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdateSkillsInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.UpdateSkills(in.Name, in.Skills); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_update_skills", in.Name, strings.Join(in.Skills, ","))
	return textResult(fmt.Sprintf("skills replaced for profile %q", in.Name)), map[string]any{"updated": in.Name}, nil
}

func (s *Server) handleProfileAddSkills(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdateSkillsInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.AddSkills(in.Name, in.Skills); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_add_skills", in.Name, strings.Join(in.Skills, ","))
	return textResult(fmt.Sprintf("skills added to profile %q", in.Name)), map[string]any{"updated": in.Name, "skills": in.Skills}, nil
}

func (s *Server) handleProfileRemoveSkills(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdateSkillsInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.RemoveSkills(in.Name, in.Skills); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_remove_skills", in.Name, strings.Join(in.Skills, ","))
	return textResult(fmt.Sprintf("skills removed from profile %q", in.Name)), map[string]any{"updated": in.Name, "skills": in.Skills}, nil
}

func (s *Server) handleProfileUpdateModel(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdateModelInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.UpdateModel(in.Name, in.Model); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_update_model", in.Name, in.Model)
	return textResult(fmt.Sprintf("model updated for profile %q", in.Name)), map[string]any{"updated": in.Name, "model": in.Model}, nil
}

func (s *Server) handleProfileUpdateMCP(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdateMCPInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.UpdateMCPServers(in.Name, in.MCPServers); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_update_mcp", in.Name, strings.Join(in.MCPServers, ","))
	return textResult(fmt.Sprintf("mcp_servers updated for profile %q", in.Name)), map[string]any{"updated": in.Name, "mcp_servers": in.MCPServers}, nil
}

func (s *Server) handleProfileUpdateDesc(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileUpdateDescInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.UpdateDescription(in.Name, in.Description); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("profile_update_desc", in.Name, "")
	return textResult(fmt.Sprintf("description updated for profile %q", in.Name)), map[string]any{"updated": in.Name}, nil
}

func (s *Server) handleProfileDelete(ctx context.Context, _ *sdkmcp.CallToolRequest, in ProfileNameInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.profileManager.Delete(in.Name); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(fmt.Sprintf("profile %q deleted", in.Name)), map[string]any{"deleted": in.Name}, nil
}

// --- Stack check handlers ---

type StackDeepInput struct {
	Workspace string `json:"workspace,omitempty" jsonschema:"workspace directory for bob/cortex probes; defaults to cwd"`
	Stash     bool   `json:"stash,omitempty" jsonschema:"if true, save the report to fcheap with minerva-stack tags"`
}

func (s *Server) handleStackCheck(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	status := monitor.CheckStack()
	return textResult(status), status, nil
}

func (s *Server) handleStackDeep(ctx context.Context, _ *sdkmcp.CallToolRequest, in StackDeepInput) (*sdkmcp.CallToolResult, any, error) {
	workspace := in.Workspace
	if workspace == "" {
		workspace = "."
	}
	deep := integration.DeepCheck(ctx, workspace)
	result := map[string]any{"status": deep}
	if in.Stash {
		outcome := "pass"
		if !deep.RetrievalReady {
			outcome = "fail"
		}
		extra := []string{}
		if deep.RetrievalReady {
			extra = append(extra, "retrieval:ready")
		} else {
			extra = append(extra, "retrieval:not-ready")
			for _, g := range deep.RetrievalGaps {
				extra = append(extra, "gap:"+g)
			}
		}
		res, err := evidence.SaveJSON(ctx, "stack-deep", "stack", outcome, extra, deep)
		if err != nil {
			result["stash_error"] = err.Error()
		} else if res != nil {
			result["stash_id"] = res.ID
			result["stash_outcome"] = outcome
			_ = s.analyticsStore.Record("stack_deep_stash", res.ID, outcome)
		}
	}
	return textResult(result), result, nil
}

type StatusInput struct {
	Workspace       string `json:"workspace,omitempty" jsonschema:"workspace for deep/suggest probes"`
	Deep            *bool  `json:"deep,omitempty" jsonschema:"include stack deep; default true"`
	IncludeEvidence *bool  `json:"include_evidence,omitempty" jsonschema:"count open fails; default true"`
	IncludeSuggest  *bool  `json:"include_suggest,omitempty" jsonschema:"include top next actions; default true"`
	MaxNext         int    `json:"max_next,omitempty" jsonschema:"max next actions; default 5"`
}

func (s *Server) handleStatus(ctx context.Context, _ *sdkmcp.CallToolRequest, in StatusInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.skillManager.LoadAll()
	_ = s.profileManager.LoadAll()
	ws := in.Workspace
	if ws == "" {
		ws, _ = os.Getwd()
		if ws == "" {
			ws = "."
		}
	}
	deep := false
	if in.Deep != nil {
		deep = *in.Deep
	}
	incEv := true
	if in.IncludeEvidence != nil {
		incEv = *in.IncludeEvidence
	}
	incSug := true
	if in.IncludeSuggest != nil {
		incSug = *in.IncludeSuggest
	}
	rep := status.Build(ctx, s.skillManager, s.profileManager, status.Options{
		Workspace:       ws,
		Deep:            deep,
		IncludeEvidence: incEv,
		IncludeSuggest:  incSug,
		MaxNextActions:  in.MaxNext,
	})
	return textResult(rep), rep, nil
}

// --- Analytics handler ---

func (s *Server) handleAnalytics(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	summary := s.analyticsStore.Summarize()
	return textResult(summary), summary, nil
}

// --- Suggest handler ---

func (s *Server) handleSuggest(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	// Reload so disk edits from CLI are visible.
	_ = s.skillManager.LoadAll()
	_ = s.profileManager.LoadAll()
	_ = s.analyticsStore.Load()

	ws, err := os.Getwd()
	if err != nil {
		ws = "."
	}
	engine := suggest.NewEngine(s.skillManager, s.profileManager, s.analyticsStore, ws)
	engine.IncludeReadiness = true
	engine.IncludeEvidence = true
	suggestions := engine.Analyze()
	return textResult(suggestions), map[string]any{"suggestions": suggestions}, nil
}

// --- Template handlers ---

type TemplateNameInput struct {
	Name string `json:"name" jsonschema:"required, template name"`
}

func (s *Server) templateDirs() []string {
	return []string{templates.DefaultDir(s.agentsDir)}
}

func (s *Server) handleTemplateList(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	all, err := templates.Catalog(s.templateDirs()...)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	// Keep MCP payload light: omit full prompts in list.
	type entry struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Role        string   `json:"role"`
		Skills      []string `json:"skills"`
		Source      string   `json:"source,omitempty"`
	}
	out := make([]entry, 0, len(all))
	for _, t := range all {
		out = append(out, entry{Name: t.Name, Description: t.Description, Role: t.Role, Skills: t.Skills, Source: t.Source})
	}
	return textResult(out), out, nil
}

func (s *Server) handleTemplateShow(ctx context.Context, _ *sdkmcp.CallToolRequest, in TemplateNameInput) (*sdkmcp.CallToolResult, any, error) {
	t := templates.GetFrom(in.Name, s.templateDirs()...)
	if t == nil {
		return errorResult(fmt.Sprintf("template %q not found; available: %s", in.Name, strings.Join(templates.NamesFrom(s.templateDirs()...), ", "))), nil, nil
	}
	return textResult(t), t, nil
}

type TemplateApplyInput struct {
	Name    string `json:"name" jsonschema:"required, template name"`
	Profile string `json:"profile,omitempty" jsonschema:"profile name; defaults to template name"`
}

func (s *Server) handleTemplateApply(ctx context.Context, _ *sdkmcp.CallToolRequest, in TemplateApplyInput) (*sdkmcp.CallToolResult, any, error) {
	t := templates.GetFrom(in.Name, s.templateDirs()...)
	if t == nil {
		return errorResult(fmt.Sprintf("template %q not found; available: %s", in.Name, strings.Join(templates.NamesFrom(s.templateDirs()...), ", "))), nil, nil
	}
	name := strings.TrimSpace(in.Profile)
	if name == "" {
		name = t.Name
	}
	_ = s.profileManager.LoadAll()
	existing := s.profileManager.Get(name)
	created := false
	if existing != nil {
		if err := s.profileManager.UpdateSystemPrompt(name, t.Prompt); err != nil {
			return errorResult(err.Error()), nil, nil
		}
		if err := s.profileManager.UpdateSkills(name, t.Skills); err != nil {
			return errorResult(err.Error()), nil, nil
		}
	} else {
		p := &profile.Profile{
			Name:         name,
			Description:  t.Description,
			Skills:       t.Skills,
			SystemPrompt: t.Prompt,
		}
		if err := s.profileManager.Create(p); err != nil {
			return errorResult(err.Error()), nil, nil
		}
		created = true
	}
	_ = s.analyticsStore.Record("template_apply", t.Name, name)
	result := map[string]any{"template": t.Name, "profile": name, "created": created}
	return textResult(result), result, nil
}

// --- Library handlers ---

type LibraryExportInput struct {
	Dest             string `json:"dest" jsonschema:"required, destination directory or .tar.gz path"`
	Note             string `json:"note,omitempty"`
	IncludeTemplates *bool  `json:"include_templates,omitempty" jsonschema:"default true"`
}

type LibraryImportInput struct {
	Source           string `json:"source" jsonschema:"required, bundle directory or .tar.gz"`
	Force            bool   `json:"force,omitempty"`
	IncludeTemplates *bool  `json:"include_templates,omitempty" jsonschema:"default true"`
}

func (s *Server) handleLibraryLint(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	rep, err := library.Lint(s.agentsDir)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(rep), rep, nil
}

func (s *Server) handleLibraryExport(ctx context.Context, _ *sdkmcp.CallToolRequest, in LibraryExportInput) (*sdkmcp.CallToolResult, any, error) {
	inc := true
	if in.IncludeTemplates != nil {
		inc = *in.IncludeTemplates
	}
	res, err := library.Export(library.ExportOptions{
		AgentsDir: s.agentsDir, Dest: in.Dest, IncludeTemplates: inc, Note: in.Note,
	})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(res), res, nil
}

func (s *Server) handleLibraryImport(ctx context.Context, _ *sdkmcp.CallToolRequest, in LibraryImportInput) (*sdkmcp.CallToolResult, any, error) {
	inc := true
	if in.IncludeTemplates != nil {
		inc = *in.IncludeTemplates
	}
	res, err := library.Import(library.ImportOptions{
		Source: in.Source, AgentsDir: s.agentsDir, Force: in.Force, IncludeTemplates: inc,
	})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	// Reload managers after import
	_ = s.skillManager.LoadAll()
	_ = s.profileManager.LoadAll()
	return textResult(res), res, nil
}

// --- Bridge handlers ---

type BridgeShowInput struct {
	Profile string `json:"profile" jsonschema:"required, profile name"`
	Format  string `json:"format,omitempty" jsonschema:"md|shell|yaml"`
	Harness string `json:"harness,omitempty" jsonschema:"harness name; default local-agent"`
}

func (s *Server) handleBridgeShow(ctx context.Context, _ *sdkmcp.CallToolRequest, in BridgeShowInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.profileManager.LoadAll()
	p := s.profileManager.Get(in.Profile)
	if p == nil {
		return errorResult(fmt.Sprintf("profile %q not found", in.Profile)), nil, nil
	}
	fmtFormat := bridge.FormatMarkdown
	switch strings.ToLower(in.Format) {
	case "shell", "sh", "bash":
		fmtFormat = bridge.FormatShell
	case "yaml", "yml":
		fmtFormat = bridge.FormatYAML
	case "md", "markdown", "":
		fmtFormat = bridge.FormatMarkdown
	default:
		return errorResult(fmt.Sprintf("unknown format %q", in.Format)), nil, nil
	}
	snip, err := bridge.Render(p, bridge.Options{
		AgentsDir: s.agentsDir, ProfileName: p.Name, Harness: in.Harness, MinervaBinary: "minerva",
	}, fmtFormat)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(snip), snip, nil
}

// --- Evidence handlers ---

type EvidenceSaveInput struct {
	Path    string   `json:"path" jsonschema:"required, file or directory to stash"`
	Name    string   `json:"name,omitempty" jsonschema:"display name"`
	Kind    string   `json:"kind,omitempty" jsonschema:"eval|suggest|stack|incident|other"`
	Outcome string   `json:"outcome,omitempty" jsonschema:"pass|fail|skip"`
	Tags    []string `json:"tags,omitempty" jsonschema:"extra tags"`
	TTL     string   `json:"ttl,omitempty" jsonschema:"e.g. 30d"`
	Index   *bool    `json:"index,omitempty" jsonschema:"index for search after save; default true"`
}

type EvidenceSearchInput struct {
	Query string `json:"query,omitempty" jsonschema:"search query; defaults to minerva"`
}

type EvidenceCloseInput struct {
	StashID string `json:"stash_id" jsonschema:"required, fail stash id to close"`
	Note    string `json:"note,omitempty" jsonschema:"optional resolution note"`
	Kind    string `json:"kind,omitempty" jsonschema:"eval|suggest|stack|incident|other"`
}

func (s *Server) handleEvidenceDocs(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	docs := evidence.Docs()
	return textResult(docs), map[string]any{"docs": docs}, nil
}

func (s *Server) handleEvidenceSearch(ctx context.Context, _ *sdkmcp.CallToolRequest, in EvidenceSearchInput) (*sdkmcp.CallToolResult, any, error) {
	out, err := evidence.SearchMinerva(ctx, in.Query)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(out), map[string]any{"raw": out}, nil
}

func (s *Server) handleEvidenceSave(ctx context.Context, _ *sdkmcp.CallToolRequest, in EvidenceSaveInput) (*sdkmcp.CallToolResult, any, error) {
	index := true
	if in.Index != nil {
		index = *in.Index
	}
	kind := in.Kind
	if kind == "" {
		kind = "eval"
	}
	res, err := evidence.Save(ctx, evidence.SaveRequest{
		Path:    in.Path,
		Name:    in.Name,
		Tags:    in.Tags,
		Kind:    kind,
		Outcome: in.Outcome,
		TTL:     in.TTL,
		Index:   index,
	})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("evidence_save", res.ID, kind)
	return textResult(res), res, nil
}

func (s *Server) handleEvidenceClose(ctx context.Context, _ *sdkmcp.CallToolRequest, in EvidenceCloseInput) (*sdkmcp.CallToolResult, any, error) {
	res, err := evidence.Close(ctx, evidence.CloseRequest{
		StashID: in.StashID,
		Note:    in.Note,
		Kind:    in.Kind,
	})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.analyticsStore.Record("evidence_close", res.ClosedID, res.ReceiptID)
	return textResult(res), res, nil
}

type LearnInput struct {
	Workspace string `json:"workspace,omitempty"`
}

func (s *Server) handleLearn(ctx context.Context, _ *sdkmcp.CallToolRequest, in LearnInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.skillManager.LoadAll()
	_ = s.profileManager.LoadAll()
	ws := in.Workspace
	if ws == "" {
		ws, _ = os.Getwd()
		if ws == "" {
			ws = "."
		}
	}
	brief := learn.Build(s.skillManager, s.profileManager, ws)
	return textResult(brief), brief, nil
}

type ResolveSkillInput struct {
	Query     string `json:"query" jsonschema:"required, natural-language task intent"`
	Workspace string `json:"workspace,omitempty"`
}

func (s *Server) handleResolveSkill(ctx context.Context, _ *sdkmcp.CallToolRequest, in ResolveSkillInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.skillManager.LoadAll()
	_ = s.profileManager.LoadAll()
	res := skill.Resolve(in.Query, s.skillManager.All(), s.profileManager.All())
	return textResult(res), res, nil
}

type SkillActionInput struct {
	Action     string `json:"action,omitempty" jsonschema:"list, show, or compare; default list"`
	Name       string `json:"name,omitempty"`
	NameA      string `json:"name_a,omitempty"`
	NameB      string `json:"name_b,omitempty"`
	SideBySide bool   `json:"side_by_side,omitempty"`
}

func (s *Server) handleSkill(ctx context.Context, req *sdkmcp.CallToolRequest, in SkillActionInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.skillManager.LoadAll()
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "", "list":
		return s.handleSkillList(ctx, req, struct{}{})
	case "show":
		return s.handleSkillShow(ctx, req, SkillNameInput{Name: in.Name})
	case "compare":
		return s.handleSkillCompare(ctx, req, SkillCompareInput{NameA: in.NameA, NameB: in.NameB, SideBySide: in.SideBySide})
	default:
		return errorResult("action must be list, show, or compare"), nil, nil
	}
}

type ProfileActionInput struct {
	Action     string `json:"action,omitempty" jsonschema:"list, show, or compare; default list"`
	Name       string `json:"name,omitempty"`
	NameA      string `json:"name_a,omitempty"`
	NameB      string `json:"name_b,omitempty"`
	SideBySide bool   `json:"side_by_side,omitempty"`
}

func (s *Server) handleProfile(ctx context.Context, req *sdkmcp.CallToolRequest, in ProfileActionInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.profileManager.LoadAll()
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "", "list":
		return s.handleProfileList(ctx, req, struct{}{})
	case "show":
		return s.handleProfileShow(ctx, req, ProfileNameInput{Name: in.Name})
	case "compare":
		return s.handleProfileCompare(ctx, req, ProfileCompareInput{NameA: in.NameA, NameB: in.NameB, SideBySide: in.SideBySide})
	default:
		return errorResult("action must be list, show, or compare"), nil, nil
	}
}

type LibraryActionInput struct {
	Action string `json:"action,omitempty" jsonschema:"lint; default lint"`
}

func (s *Server) handleLibrary(ctx context.Context, req *sdkmcp.CallToolRequest, in LibraryActionInput) (*sdkmcp.CallToolResult, any, error) {
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "", "lint":
		return s.handleLibraryLint(ctx, req, struct{}{})
	default:
		return errorResult("action must be lint (export/import are CLI-only)"), nil, nil
	}
}

type EvidenceActionInput struct {
	Action string `json:"action,omitempty" jsonschema:"docs or search; default docs"`
	Query  string `json:"query,omitempty"`
}

func (s *Server) handleEvidence(ctx context.Context, req *sdkmcp.CallToolRequest, in EvidenceActionInput) (*sdkmcp.CallToolResult, any, error) {
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "", "docs":
		return s.handleEvidenceDocs(ctx, req, struct{}{})
	case "search":
		return s.handleEvidenceSearch(ctx, req, EvidenceSearchInput{Query: in.Query})
	default:
		return errorResult("action must be docs or search (save/close are CLI-only)"), nil, nil
	}
}

// --- Helpers ---

func textResult(v any) *sdkmcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return &sdkmcp.CallToolResult{
			IsError: true,
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: fmt.Sprintf("marshal error: %v", err)}},
		}
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}
}

func errorResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: msg}},
	}
}
