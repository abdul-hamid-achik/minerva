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
	"github.com/abdul-hamid-achik/minerva/internal/evidence"
	"github.com/abdul-hamid-achik/minerva/internal/integration"
	"github.com/abdul-hamid-achik/minerva/internal/monitor"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/suggest"
	"github.com/abdul-hamid-achik/minerva/internal/templates"
	"github.com/abdul-hamid-achik/minerva/internal/version"
)

const instructions = `Minerva is the agent library operator for ~/.agents (skills + profiles + templates)
and a stack readiness orchestrator. It is NOT a second agent runtime.

- Skill/profile CRUD writes the same disk layout local-agent loads.
- minerva_skill_activate updates Minerva-local activation state only; it does NOT
  inject skills into a live local-agent session. Prefer profile skill lists for
  durable harness behavior.
- minerva_stack_check = presence (correct binaries, tiered health).
- minerva_stack_deep = bob/cortex/mcphub + readiness doctors/status.
- minerva_suggest returns ranked actions; auto-apply is CLI-only for activate.

Do not reimplement MCPHub gateway, Cortex tasks, or Bob apply through Minerva.`

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
	// Skill management
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_skill_list", "List all available skills",
		"Return all discovered skills with their name, description, active state, and path.",
	), s.handleSkillList)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_skill_show", "Show a skill's full content",
		"Return the complete markdown body of a skill by name.",
	), s.handleSkillShow)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_skill_compare", "Compare two skills side by side",
		"Return the content of two skills for comparison.",
	), s.handleSkillCompare)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_skill_create",
		Title:       "Create a new skill",
		Description: "Create a new skill with a name, description, and markdown content. The skill is written to the skills directory.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Create a new skill", ReadOnlyHint: false},
	}, s.handleSkillCreate)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_skill_activate",
		Title:       "Activate a skill (Minerva-local)",
		Description: "Mark a skill active in Minerva's local state (~/.agents/.minerva-skills.json). Does not inject into a live local-agent session; use profile skills for durable harness loading.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Activate a skill (Minerva-local)", ReadOnlyHint: false},
	}, s.handleSkillActivate)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_skill_deactivate",
		Title:       "Deactivate a skill (Minerva-local)",
		Description: "Clear Minerva-local activation for a skill. Does not change a live local-agent session.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Deactivate a skill (Minerva-local)", ReadOnlyHint: false},
	}, s.handleSkillDeactivate)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_skill_delete",
		Title:       "Delete a skill",
		Description: "Permanently delete a skill and its directory.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Delete a skill", ReadOnlyHint: false},
	}, s.handleSkillDelete)

	// Profile management
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_profile_list", "List all agent profiles",
		"Return all discovered agent profiles with their name, description, model, skills, and MCP server allowlists.",
	), s.handleProfileList)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_profile_show", "Show a profile's full configuration",
		"Return the complete configuration of an agent profile by name, including its system prompt.",
	), s.handleProfileShow)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_profile_compare", "Compare two profiles side by side",
		"Return the full configuration of two profiles for comparison.",
	), s.handleProfileCompare)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_profile_create",
		Title:       "Create a new agent profile",
		Description: "Create a new agent profile with name, description, model, skills, MCP servers, and system prompt.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Create a new agent profile", ReadOnlyHint: false},
	}, s.handleProfileCreate)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_profile_update_prompt",
		Title:       "Update a profile's system prompt",
		Description: "Update the system prompt for an existing agent profile.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Update a profile's system prompt", ReadOnlyHint: false},
	}, s.handleProfileUpdatePrompt)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_profile_update_skills",
		Title:       "Update a profile's skills",
		Description: "Update the skills list for an existing agent profile.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Update a profile's skills", ReadOnlyHint: false},
	}, s.handleProfileUpdateSkills)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_profile_delete",
		Title:       "Delete an agent profile",
		Description: "Permanently delete an agent profile and its directory.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Delete an agent profile", ReadOnlyHint: false},
	}, s.handleProfileDelete)

	// Stack monitoring
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_stack_check", "Check stack presence (tiered)",
		"Probe PATH for intelligence-stack tools using real binary names (glyph, cairn, tvault). Core missing → unhealthy; optional missing → degraded. Not domain readiness.",
	), s.handleStackCheck)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_stack_deep", "Deep stack readiness probe",
		"Compose bob check/context, cortex doctor, mcphub stats, and optional readiness probes (codemap/vecgrep/fcheap/tvault/monitor). Workspace defaults to cwd.",
	), s.handleStackDeep)

	// Analytics
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_analytics", "View usage analytics",
		"Return Minerva-local usage analytics (skill activations, profile events). Not mcphub tool_calls.",
	), s.handleAnalytics)

	// Suggestions
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_suggest", "Get library and stack suggestions",
		"Ranked suggestions from skills, profiles, stack presence/readiness, mcphub stats, analytics, and workspace type. Activation is Minerva-local only.",
	), s.handleSuggest)

	// Templates (local-agent trust lists already expect these names)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_template_list", "List system prompt templates",
		"Return built-in role templates (name, description, role, recommended skills).",
	), s.handleTemplateList)
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_template_show", "Show a template",
		"Return a template's full system prompt and recommended skills by name.",
	), s.handleTemplateShow)

	// Evidence via fcheap conventions
	sdkmcp.AddTool(s.srv, readOnlyTool(
		"minerva_evidence_docs", "Minerva fcheap tag conventions",
		"Return the standard tag scheme for stashing Minerva eval/stack outcomes in fcheap.",
	), s.handleEvidenceDocs)
	sdkmcp.AddTool(s.srv, &sdkmcp.Tool{
		Name:        "minerva_evidence_save",
		Title:       "Save evidence via fcheap",
		Description: "Stash a file/directory with Minerva tags (minerva, minerva-eval, outcome:pass/fail, …) using fcheap. Does not store secrets.",
		Annotations: &sdkmcp.ToolAnnotations{Title: "Save evidence via fcheap", ReadOnlyHint: false},
	}, s.handleEvidenceSave)
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
	NameA string `json:"name_a" jsonschema:"required, first skill name"`
	NameB string `json:"name_b" jsonschema:"required, second skill name"`
}

type SkillCreateInput struct {
	Name        string `json:"name" jsonschema:"required, unique skill name"`
	Description string `json:"description,omitempty" jsonschema:"one-line description of what the skill does"`
	Content     string `json:"content" jsonschema:"required, markdown body of the skill"`
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
	result := map[string]any{
		"skill_a": map[string]any{"name": in.NameA, "content": contentA},
		"skill_b": map[string]any{"name": in.NameB, "content": contentB},
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
	NameA string `json:"name_a" jsonschema:"required, first profile name"`
	NameB string `json:"name_b" jsonschema:"required, second profile name"`
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

func (s *Server) handleProfileList(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	profiles := s.profileManager.All()
	return textResult(profiles), profiles, nil
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
	result := map[string]any{
		"profile_a": pA,
		"profile_b": pB,
	}
	return textResult(result), result, nil
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
	return textResult(fmt.Sprintf("skills updated for profile %q", in.Name)), map[string]any{"updated": in.Name}, nil
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
	status := integration.DeepCheck(ctx, workspace)
	result := map[string]any{"status": status}
	if in.Stash {
		outcome := "pass"
		if !status.RetrievalReady {
			outcome = "fail"
		}
		extra := []string{}
		if status.RetrievalReady {
			extra = append(extra, "retrieval:ready")
		} else {
			extra = append(extra, "retrieval:not-ready")
			for _, g := range status.RetrievalGaps {
				extra = append(extra, "gap:"+g)
			}
		}
		res, err := evidence.SaveJSON(ctx, "stack-deep", "stack", outcome, extra, status)
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

func (s *Server) handleTemplateList(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	all := templates.All()
	// Keep MCP payload light: omit full prompts in list.
	type entry struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Role        string   `json:"role"`
		Skills      []string `json:"skills"`
	}
	out := make([]entry, 0, len(all))
	for _, t := range all {
		out = append(out, entry{Name: t.Name, Description: t.Description, Role: t.Role, Skills: t.Skills})
	}
	return textResult(out), out, nil
}

func (s *Server) handleTemplateShow(ctx context.Context, _ *sdkmcp.CallToolRequest, in TemplateNameInput) (*sdkmcp.CallToolResult, any, error) {
	t := templates.Get(in.Name)
	if t == nil {
		return errorResult(fmt.Sprintf("template %q not found; available: %s", in.Name, strings.Join(templates.Names(), ", "))), nil, nil
	}
	return textResult(t), t, nil
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

func (s *Server) handleEvidenceDocs(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	docs := evidence.Docs()
	return textResult(docs), map[string]any{"docs": docs}, nil
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
