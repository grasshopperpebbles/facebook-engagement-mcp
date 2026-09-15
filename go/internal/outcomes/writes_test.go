package outcomes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// T-26, 2026-09-08. pageId was optional and the tool fell back to the startup
// client, which holds the USER token — so a caller supplying only a comment id
// got Meta's refusal naming publish_actions, a permission removed in 2018. The
// message is literally true and entirely misleading: the fault is ours.
func TestReplyWithoutAPageIDIsRefusedAndReachesGraphNotAtAll(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})

	_, err := RunRespondToComment(context.Background(), s.deps(t), RespondOptions{
		CommentID: "c1", Message: "hello",
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "pageId") {
		t.Fatalf("refusal does not name the field: %v", err)
	}
	// And it explains the misleading message the caller would otherwise get.
	if !strings.Contains(err.Error(), "publish_actions") {
		t.Errorf("refusal does not warn about the dead permission: %v", err)
	}
	if len(s.paths) != 0 {
		t.Fatalf("%d requests reached Graph; want 0: %v", len(s.paths), s.paths)
	}
}

func TestModerateWithoutAPageIDIsRefused(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})

	_, err := RunModerateComment(context.Background(), s.deps(t), ModerateOptions{
		CommentID: "c1", Action: ActionHide,
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "pageId") {
		t.Fatalf("refusal does not name the field: %v", err)
	}
	if len(s.paths) != 0 {
		t.Fatalf("%d requests reached Graph; want 0", len(s.paths))
	}
}

// The dry run is checked AFTER the pageId guard, so it can never report that a
// call would work when it could not.
func TestDryRunIsCheckedAfterThePageIDGuard(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})

	_, err := RunRespondToComment(context.Background(), s.deps(t), RespondOptions{
		CommentID: "c1", Message: "hello", DryRun: true,
	})
	if err == nil {
		t.Fatal("a dry run with no pageId reported success; it must refuse first")
	}

	_, err = RunModerateComment(context.Background(), s.deps(t), ModerateOptions{
		CommentID: "c1", Action: ActionHide, DryRun: true,
	})
	if err == nil {
		t.Fatal("a moderation dry run with no pageId reported success")
	}
}

// A reply is public the moment it exists and deleting it does not unpublish what
// people already saw, so the dry run is the only safe way to confirm a target.
func TestDryRunPublishesNothing(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})

	out, err := RunRespondToComment(context.Background(), s.deps(t), RespondOptions{
		CommentID: "c1", PageID: "pg1", Message: "hello", DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(s.paths) != 0 {
		t.Fatalf("a dry run made %d requests: %v", len(s.paths), s.paths)
	}

	encoded, _ := json.Marshal(out)
	// It reports the SHAPE of the effect, never the text — the caller wrote it,
	// and echoing it back costs context for nothing.
	if strings.Contains(string(encoded), "hello") {
		t.Fatalf("the dry run echoed the caller's text back: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"dryRun":true`) {
		t.Fatalf("the dry run did not mark itself: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"messageLength":5`) {
		t.Fatalf("the dry run did not report the message length: %s", encoded)
	}
}

func TestModerateDryRunChangesNothing(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})

	out, err := RunModerateComment(context.Background(), s.deps(t), ModerateOptions{
		CommentID: "c1", PageID: "pg1", Action: ActionHide, DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(s.paths) != 0 {
		t.Fatalf("a dry run made %d requests: %v", len(s.paths), s.paths)
	}
	encoded, _ := json.Marshal(out)
	if !strings.Contains(string(encoded), `"dryRun":true`) {
		t.Fatalf("the dry run did not mark itself: %s", encoded)
	}
}

func TestAnEmptyReplyIsRefused(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})

	for _, message := range []string{"", "   ", "\n\t "} {
		if _, err := RunRespondToComment(context.Background(), s.deps(t), RespondOptions{
			CommentID: "c1", PageID: "pg1", Message: message,
		}); err == nil {
			t.Errorf("an empty reply (%q) was accepted", message)
		}
	}
	if len(s.paths) != 0 {
		t.Fatalf("%d requests reached Graph for an empty reply", len(s.paths))
	}
}

// The reply goes out on a PAGE token, exchanged for the pageId given.
func TestAReplyPublishesOnThePageToken(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts": accountsWithPage,
		"/c1/comments": `{"id":"c1_r1"}`,
	})

	out, err := RunRespondToComment(context.Background(), s.deps(t), RespondOptions{
		CommentID: "c1", PageID: "pg1", Message: "Thanks for asking",
	})
	if err != nil {
		t.Fatalf("RunRespondToComment: %v", err)
	}
	if out["replyId"] != "c1_r1" {
		t.Fatalf("out = %+v", out)
	}
	if !s.requested("/c1/comments") {
		t.Fatal("the reply was never sent")
	}
}

func TestModerationSendsTheAction(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts": accountsWithPage,
		"/c1":          `{"success":true}`,
	})

	out, err := RunModerateComment(context.Background(), s.deps(t), ModerateOptions{
		CommentID: "c1", PageID: "pg1", Action: ActionUnhide,
	})
	if err != nil {
		t.Fatalf("RunModerateComment: %v", err)
	}
	if out["hidden"] != false || out["action"] != "unhide" {
		t.Fatalf("out = %+v", out)
	}
}

func TestAnUnknownActionIsRefused(t *testing.T) {
	s := newStub(t, map[string]string{"/me/accounts": accountsWithPage})
	if _, err := RunModerateComment(context.Background(), s.deps(t), ModerateOptions{
		CommentID: "c1", PageID: "pg1", Action: "delete",
	}); err == nil {
		t.Fatal("an unknown action was accepted; this server cannot delete comments")
	}
}
