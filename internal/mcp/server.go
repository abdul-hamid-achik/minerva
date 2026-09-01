// Package mcp exposes Minerva over the Model Context Protocol (stdio).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/learn"
	"github.com/abdul-hamid-achik/minerva/internal/propose"
	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/signal"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/surface"
	"github.com/abdul-hamid-achik/minerva/internal/sync"
	"github.com/abdul-hamid-achik/minerva/internal/textdiff"
	"github.com/abdul-hamid-achik/minerva/internal/version"
)

const instructions = `Minerva reads agent-harness conversations and tool calls, proposes skills, and syncs SKILL.md libraries.
It is NOT a second agent runtime and does not monitor companion CLIs.

- minerva_learn first if you do not know the contract.
- minerva_sessions / minerva_analyze / minerva_propose to turn traces into skill drafts.
- minerva_apply writes a proposal to ~/.agents/skills (approval-gated).
- minerva_resolve_skill picks a catalog skill for the current task.`

// Server wraps the go-sdk MCP server.
type Server struct {
	skillManager *skill.Manager
	agentsDir    string
	srv          *sdkmcp.Server
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
	skillMgr := skill.ForAgents(agentsDir)
	if err := skillMgr.LoadAll(); err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	s := &Server{skillManager: skillMgr, agentsDir: agentsDir}
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
		spec := toolSpec(tool)
		switch tool.Name {
		case "minerva_learn":
			sdkmcp.AddTool(s.srv, spec, s.handleLearn)
		case "minerva_sessions":
			sdkmcp.AddTool(s.srv, spec, s.handleSessions)
		case "minerva_analyze":
			sdkmcp.AddTool(s.srv, spec, s.handleAnalyze)
		case "minerva_propose":
			sdkmcp.AddTool(s.srv, spec, s.handlePropose)
		case "minerva_resolve_skill":
			sdkmcp.AddTool(s.srv, spec, s.handleResolveSkill)
		case "minerva_skill":
			sdkmcp.AddTool(s.srv, spec, s.handleSkill)
		case "minerva_harness":
			sdkmcp.AddTool(s.srv, spec, s.handleHarness)
		case "minerva_apply":
			sdkmcp.AddTool(s.srv, spec, s.handleApply)
		}
	}
}

func toolSpec(tool surface.ProductTool) *sdkmcp.Tool {
	destructive := !tool.ReadOnly
	openWorld := false
	return &sdkmcp.Tool{
		Name: tool.Name, Title: tool.Title, Description: tool.Description,
		Annotations: &sdkmcp.ToolAnnotations{
			Title: tool.Title, ReadOnlyHint: tool.ReadOnly, DestructiveHint: &destructive,
			IdempotentHint: tool.ReadOnly, OpenWorldHint: &openWorld,
		},
	}
}

func (s *Server) env() harness.Env {
	e := harness.DefaultEnv()
	e.AgentsDir = s.agentsDir
	return e
}

func (s *Server) handleLearn(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	brief := learn.Build()
	return textResult(brief), brief, nil
}

type FilterInput struct {
	Harness   string `json:"harness,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Since     string `json:"since,omitempty" jsonschema:"age window such as 7d or 24h"`
	Limit     int    `json:"limit,omitempty"`
	Session   string `json:"session,omitempty"`
	Last      bool   `json:"last,omitempty"`
}

func (s *Server) filter(in FilterInput) (session.Filter, error) {
	dur, err := parseSince(in.Since)
	if err != nil {
		return session.Filter{}, err
	}
	limit := in.Limit
	if in.Last {
		limit = 1
	}
	if limit == 0 {
		limit = 20
	}
	return session.Filter{Harness: in.Harness, Workspace: in.Workspace, Since: dur, Limit: limit, SessionID: in.Session}, nil
}

func (s *Server) handleSessions(ctx context.Context, _ *sdkmcp.CallToolRequest, in FilterInput) (*sdkmcp.CallToolResult, any, error) {
	f, err := s.filter(in)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	list, err := session.List(s.env(), f)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return textResult(list), list, nil
}

func (s *Server) handleAnalyze(ctx context.Context, _ *sdkmcp.CallToolRequest, in FilterInput) (*sdkmcp.CallToolResult, any, error) {
	f, err := s.filter(in)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	sessions, err := session.LoadFiltered(s.env(), f)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	for i := range sessions {
		session.RedactSession(&sessions[i])
	}
	_ = s.skillManager.LoadAll()
	sigs := signal.Extract(sessions, s.skillManager.All())
	payload := map[string]any{"sessions": len(sessions), "signals": sigs}
	return textResult(payload), payload, nil
}

func (s *Server) handlePropose(ctx context.Context, _ *sdkmcp.CallToolRequest, in FilterInput) (*sdkmcp.CallToolResult, any, error) {
	if in.Since == "" {
		in.Since = "30d"
	}
	if in.Limit == 0 {
		in.Limit = 40
	}
	f, err := s.filter(in)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	sessions, err := session.LoadFiltered(s.env(), f)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	for i := range sessions {
		session.RedactSession(&sessions[i])
	}
	_ = s.skillManager.LoadAll()
	sigs := signal.Extract(sessions, s.skillManager.All())
	proposals := propose.FromSignals(sigs, s.skillManager.All())
	if err := propose.Save(s.agentsDir, proposals); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	payload := map[string]any{"sessions": len(sessions), "proposals": proposals}
	return textResult(payload), payload, nil
}

type ResolveSkillInput struct {
	Query string `json:"query" jsonschema:"required, natural-language task intent"`
}

func (s *Server) handleResolveSkill(ctx context.Context, _ *sdkmcp.CallToolRequest, in ResolveSkillInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.skillManager.LoadAll()
	res := skill.Resolve(in.Query, s.skillManager.All())
	return textResult(res), res, nil
}

type SkillActionInput struct {
	Action     string `json:"action,omitempty" jsonschema:"list, show, or compare; default list"`
	Name       string `json:"name,omitempty"`
	NameA      string `json:"name_a,omitempty"`
	NameB      string `json:"name_b,omitempty"`
	SideBySide bool   `json:"side_by_side,omitempty"`
}

func (s *Server) handleSkill(ctx context.Context, _ *sdkmcp.CallToolRequest, in SkillActionInput) (*sdkmcp.CallToolResult, any, error) {
	_ = s.skillManager.LoadAll()
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "", "list":
		catalog := s.skillManager.Catalog()
		return textResult(catalog), catalog, nil
	case "show":
		content, ok := s.skillManager.Load(in.Name)
		if !ok {
			return errorResult(fmt.Sprintf("skill %q not found", in.Name)), nil, nil
		}
		return textResult(content), map[string]any{"name": in.Name, "content": content}, nil
	case "compare":
		a, okA := s.skillManager.Load(in.NameA)
		b, okB := s.skillManager.Load(in.NameB)
		if !okA {
			return errorResult(fmt.Sprintf("skill %q not found", in.NameA)), nil, nil
		}
		if !okB {
			return errorResult(fmt.Sprintf("skill %q not found", in.NameB)), nil, nil
		}
		if in.SideBySide {
			result := map[string]any{"skill_a": a, "skill_b": b}
			return textResult(result), result, nil
		}
		diff := textdiff.Unified(in.NameA, in.NameB, a, b)
		result := map[string]any{"identical": diff == "", "diff": diff}
		return textResult(result), result, nil
	default:
		return errorResult("action must be list, show, or compare"), nil, nil
	}
}

type HarnessActionInput struct {
	Action string `json:"action,omitempty" jsonschema:"list or doctor; default list"`
}

func (s *Server) handleHarness(ctx context.Context, _ *sdkmcp.CallToolRequest, in HarnessActionInput) (*sdkmcp.CallToolResult, any, error) {
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "", "list":
		cat := harness.Catalog(s.env())
		return textResult(cat), cat, nil
	case "doctor":
		findings, err := sync.Doctor(s.env())
		if err != nil {
			return errorResult(err.Error()), nil, nil
		}
		return textResult(findings), findings, nil
	default:
		return errorResult("action must be list or doctor"), nil, nil
	}
}

type ApplyInput struct {
	ProposalID string `json:"proposal_id" jsonschema:"required, id from minerva_propose"`
}

func (s *Server) handleApply(ctx context.Context, _ *sdkmcp.CallToolRequest, in ApplyInput) (*sdkmcp.CallToolResult, any, error) {
	p, err := propose.Find(s.agentsDir, in.ProposalID)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	path, err := propose.Apply(s.agentsDir, *p)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	_ = s.skillManager.LoadAll()
	result := map[string]any{"applied": p.ID, "path": path, "name": p.Name}
	return textResult(result), result, nil
}

func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if strings.HasSuffix(s, "d") {
		n := strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(n, "%d", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("invalid since %q", s)
}

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
