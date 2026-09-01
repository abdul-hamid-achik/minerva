package skill

import "testing"

func testSkill(name, desc string) *Skill {
	return &Skill{Name: name, Description: desc, Content: desc}
}

func TestResolve_RanksReviewAndDocs(t *testing.T) {
	skills := []*Skill{
		testSkill("qa-tester", "Use when writing test plans, test cases, bug reports"),
		testSkill("differential-review", "Performs security-focused differential review of PRs and diffs"),
		testSkill("doc-writer", "Use when writing or editing READMEs and API docs"),
		testSkill("frontend-dev", "Use when building frontend pages and React sites"),
		testSkill("stripe-docs", "Look up Stripe documentation or API reference"),
		testSkill("ship", "Use when committing, pushing a branch, or opening a pull request"),
	}

	review := Resolve("review this pull request", skills)
	if len(review.Hits) == 0 {
		t.Fatal("expected review hits")
	}

	docs := Resolve("write the readme documentation", skills)
	if len(docs.Hits) == 0 || docs.Hits[0].Name != "doc-writer" {
		t.Fatalf("docs hits=%#v", docs.Hits)
	}

	front := Resolve("build a react frontend ui", skills)
	if len(front.Hits) == 0 || front.Hits[0].Name != "frontend-dev" {
		t.Fatalf("frontend hits=%#v", front.Hits)
	}

	stripe := Resolve("look up stripe payments api", skills)
	if len(stripe.Hits) == 0 || stripe.Hits[0].Name != "stripe-docs" {
		t.Fatalf("stripe hits=%#v", stripe.Hits)
	}

	testingQ := Resolve("need a qa testing strategy", skills)
	if len(testingQ.Hits) == 0 || testingQ.Hits[0].Name != "qa-tester" {
		t.Fatalf("testing hits=%#v", testingQ.Hits)
	}
}

func TestResolve_EmptyQuery(t *testing.T) {
	got := Resolve("   ", nil)
	if len(got.Hits) != 0 {
		t.Fatalf("%#v", got)
	}
}
