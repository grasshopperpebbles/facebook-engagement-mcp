package graph

import (
	"encoding/json"
	"testing"
)

// Go has no undefined. A zero struct is a struct, so an absent author and an
// author with an empty id must not become the same value — statusBasis turns
// entirely on whether an author was present ANYWHERE.
func TestAbsentAuthorIsNilNotZero(t *testing.T) {
	var raw RawComment
	if err := json.Unmarshal([]byte(`{"id":"c1","message":"hi"}`), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := NormalizeComment(raw)

	if got.Author != nil {
		t.Fatalf("Author = %+v, want nil — Graph did not return `from`", got.Author)
	}
	if got.ReplyCount != nil {
		t.Fatalf("ReplyCount = %v, want nil — absent is not zero", *got.ReplyCount)
	}
	if got.Hidden != nil {
		t.Fatalf("Hidden = %v, want nil — absent is not false", *got.Hidden)
	}
	if got.CanHide != nil {
		t.Fatalf("CanHide = %v, want nil — absent is not false", *got.CanHide)
	}
	if got.CanComment != nil {
		t.Fatalf("CanComment = %v, want nil — absent is not false", *got.CanComment)
	}
}

// And a returned zero is a real answer that must survive as one.
func TestReturnedZerosAreKept(t *testing.T) {
	var raw RawComment
	body := `{"id":"c1","comment_count":0,"is_hidden":false,"can_hide":false,"like_count":0}`
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := NormalizeComment(raw)

	if got.ReplyCount == nil || *got.ReplyCount != 0 {
		t.Fatalf("ReplyCount = %v, want a present 0", got.ReplyCount)
	}
	if got.Hidden == nil || *got.Hidden != false {
		t.Fatalf("Hidden = %v, want a present false", got.Hidden)
	}
	if got.CanHide == nil || *got.CanHide != false {
		t.Fatalf("CanHide = %v, want a present false", got.CanHide)
	}
	if got.LikeCount == nil || *got.LikeCount != 0 {
		t.Fatalf("LikeCount = %v, want a present 0", got.LikeCount)
	}
}

// An author Graph returned, with an id, is the case everything else rests on.
func TestPresentAuthorSurvives(t *testing.T) {
	var raw RawComment
	_ = json.Unmarshal([]byte(`{"id":"c1","from":{"id":"pg1","name":"Test Page"}}`), &raw)
	got := NormalizeComment(raw)

	if got.Author == nil || got.Author.ID == nil || *got.Author.ID != "pg1" {
		t.Fatalf("Author = %+v, want id pg1", got.Author)
	}
	if got.Author.Name == nil || *got.Author.Name != "Test Page" {
		t.Fatalf("Author.Name = %v, want Test Page", got.Author.Name)
	}
}

// Graph can return `from` as an object with no id. That is not an author.
func TestAuthorWithoutAnIDIsNotAnIdentity(t *testing.T) {
	var raw RawComment
	_ = json.Unmarshal([]byte(`{"id":"c1","from":{"name":"Someone"}}`), &raw)
	got := NormalizeComment(raw)

	if got.Author == nil {
		t.Fatal("Author = nil; Graph did return `from`")
	}
	if got.Author.ID != nil {
		t.Fatalf("Author.ID = %v, want nil", got.Author.ID)
	}
}

// is_published is the one field that defaults rather than staying absent: an
// omitted value must not be read as "this is an ad post".
func TestPostDefaultsToPublishedWhenGraphOmitsTheField(t *testing.T) {
	var raw RawPost
	if err := json.Unmarshal([]byte(`{"id":"p1"}`), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := NormalizePost(raw); !got.IsPublished {
		t.Fatal("IsPublished = false for a post Graph said nothing about; want true")
	}

	var unpublished RawPost
	if err := json.Unmarshal([]byte(`{"id":"p2","is_published":false}`), &unpublished); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := NormalizePost(unpublished); got.IsPublished {
		t.Fatal("IsPublished = true for a post Graph said was unpublished")
	}
}

// parent.id survives, and a parent object without an id does not invent one.
func TestParentIDIsFlattenedSafely(t *testing.T) {
	var withParent RawComment
	_ = json.Unmarshal([]byte(`{"id":"r1","parent":{"id":"c1"}}`), &withParent)
	if got := NormalizeComment(withParent); got.ParentID == nil || *got.ParentID != "c1" {
		t.Fatalf("ParentID = %v, want c1", got.ParentID)
	}

	var emptyParent RawComment
	_ = json.Unmarshal([]byte(`{"id":"r2","parent":{}}`), &emptyParent)
	if got := NormalizeComment(emptyParent); got.ParentID != nil {
		t.Fatalf("ParentID = %v, want nil", got.ParentID)
	}

	var noParent RawComment
	_ = json.Unmarshal([]byte(`{"id":"c1"}`), &noParent)
	if got := NormalizeComment(noParent); got.ParentID != nil {
		t.Fatalf("ParentID = %v, want nil", got.ParentID)
	}
}

// page_story_id is why RawPhoto exists at all: a photo can be reachable when the
// post carrying it is not, and comparing the two is the only way this client can
// notice it is being shown less than the Page holds (T-37).
func TestPhotoCarriesThePostItBelongsTo(t *testing.T) {
	var raw RawPhoto
	_ = json.Unmarshal([]byte(`{"id":"ph1","created_time":"2026-09-11T16:44:00+0000","page_story_id":"pg1_p1"}`), &raw)
	got := NormalizePhoto(raw)

	if got.PostID == nil || *got.PostID != "pg1_p1" {
		t.Fatalf("PostID = %v, want pg1_p1", got.PostID)
	}
	if got.CreatedTime == nil {
		t.Fatal("CreatedTime = nil")
	}
}

// Absent and empty are different answers for tasks. nil means Graph did not say
// what this identity may do; an empty slice means it holds no role. A caller
// refusing a write on a missing MODERATE role must act on the second, not the
// first.
func TestPageTasksDistinguishAbsentFromEmpty(t *testing.T) {
	var absent RawPage
	_ = json.Unmarshal([]byte(`{"id":"pg1","name":"Test Page"}`), &absent)
	if got := NormalizePage(absent); got.Tasks != nil {
		t.Fatalf("Tasks = %v, want nil — Graph did not return the field", *got.Tasks)
	}

	var empty RawPage
	_ = json.Unmarshal([]byte(`{"id":"pg1","tasks":[]}`), &empty)
	got := NormalizePage(empty)
	if got.Tasks == nil {
		t.Fatal("Tasks = nil; Graph returned the field, empty")
	}
	if len(*got.Tasks) != 0 {
		t.Fatalf("Tasks = %v, want empty", *got.Tasks)
	}
}
