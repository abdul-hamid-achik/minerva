package integration

import "testing"

func TestParseCodemapStatus_UnregisteredNotReady(t *testing.T) {
	raw := []byte(`{
		"project":"minerva",
		"registered":false,
		"nodes":0,
		"edges":0,
		"files":0,
		"vectors":0,
		"semantic_backend":"fallback",
		"project_key":"abc"
	}`)
	rp := parseCodemapStatus(raw, nil)
	if rp.Ready {
		t.Fatal("unregistered codemap must not be ready")
	}
	if len(rp.NextActions) == 0 {
		t.Fatal("expected next_actions for unindexed project")
	}
}

func TestParseCodemapStatus_RegisteredReady(t *testing.T) {
	raw := []byte(`{
		"registered":true,
		"nodes":120,
		"edges":400,
		"vectors":10,
		"semantic_backend":"local"
	}`)
	rp := parseCodemapStatus(raw, nil)
	if !rp.Ready {
		t.Fatalf("expected ready, got error=%q detail=%q", rp.Error, rp.Detail)
	}
}

func TestParseVecgrepStatus_EmptyIndex(t *testing.T) {
	raw := []byte(`{
		"index_fresh": false,
		"profile_matches": true,
		"provider": "ollama",
		"embedding_model": "nomic-embed-text",
		"stats": {"chunks": 0, "files": 0}
	}`)
	rp := parseVecgrepStatus(raw, nil)
	if rp.Ready {
		t.Fatal("empty vecgrep index must not be ready")
	}
	if len(rp.NextActions) == 0 {
		t.Fatal("expected index next action")
	}
}

func TestParseVecgrepStatus_Healthy(t *testing.T) {
	raw := []byte(`{
		"index_fresh": true,
		"profile_matches": true,
		"provider_health": "ok",
		"provider": "ollama",
		"embedding_model": "nomic-embed-text",
		"stats": {"chunks": 1200, "files": 80}
	}`)
	rp := parseVecgrepStatus(raw, nil)
	if !rp.Ready {
		t.Fatalf("expected ready: %q %q", rp.Error, rp.Detail)
	}
}
