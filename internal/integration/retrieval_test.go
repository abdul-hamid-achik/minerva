package integration

import "testing"

func TestComputeRetrieval_BothReady(t *testing.T) {
	s := &DeepStackStatus{
		Readiness: []ReadinessProbe{
			{Tool: "codemap", Ready: true},
			{Tool: "vecgrep", Ready: true},
			{Tool: "fcheap", Ready: true},
		},
	}
	s.computeRetrieval()
	if !s.RetrievalReady {
		t.Fatalf("expected ready, gaps=%v detail=%s", s.RetrievalGaps, s.RetrievalDetail)
	}
}

func TestComputeRetrieval_CodemapDown(t *testing.T) {
	s := &DeepStackStatus{
		Readiness: []ReadinessProbe{
			{Tool: "codemap", Ready: false, Error: "not indexed"},
			{Tool: "vecgrep", Ready: true},
		},
	}
	s.computeRetrieval()
	if s.RetrievalReady {
		t.Fatal("expected not ready")
	}
	if len(s.RetrievalGaps) != 1 || s.RetrievalGaps[0] != "codemap" {
		t.Fatalf("gaps=%v", s.RetrievalGaps)
	}
}

func TestComputeRetrieval_MissingProbe(t *testing.T) {
	s := &DeepStackStatus{
		Readiness: []ReadinessProbe{
			{Tool: "monitor", Ready: true},
		},
	}
	s.computeRetrieval()
	if s.RetrievalReady {
		t.Fatal("expected not ready when probes missing")
	}
	if len(s.RetrievalGaps) != 2 {
		t.Fatalf("gaps=%v", s.RetrievalGaps)
	}
}
