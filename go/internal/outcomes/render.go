package outcomes

import "github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"

// Declared truncation limits for free text.
const (
	// MaxCommentChars bounds a comment body.
	MaxCommentChars = 1000

	// MaxAuthorNameChars bounds a display name.
	//
	// A Facebook display name is free text chosen by whoever wrote the comment,
	// so it is exactly as untrusted as the comment body and gets a hard ceiling
	// of its own. Eighty characters is past any real name and far short of a
	// paragraph of smuggled instructions.
	MaxAuthorNameChars = 80

	// MaxResponseTextChars is the total free text one response may carry, across
	// every post body, comment and reply in it.
	//
	// The differentiation argument for this server is context economy: triaging
	// 400 comments server-side is supposed to cost the groups, not the comments.
	// Without a ceiling a default sweep of a busy Page returns a quarter of a
	// megabyte, which costs the caller both. Threads past the budget are returned
	// with their structure and without their text, and the answer says how many.
	MaxResponseTextChars = 40_000
)

// UntrustedText is free text written by a stranger, marked as such.
type UntrustedText struct {
	// Untrusted is always true. A structural marker that this value came from
	// someone outside the system.
	Untrusted bool   `json:"untrusted"`
	Value     string `json:"value"`
	Truncated bool   `json:"truncated"`
}

// RenderedAuthor is a comment's author.
//
// ID is a Graph identifier — not free text, and load-bearing for triage, so it
// is returned as-is. Name is attacker-chosen and gets the same delimited,
// truncated treatment as the comment body.
type RenderedAuthor struct {
	ID   *string        `json:"id,omitempty"`
	Name *UntrustedText `json:"name,omitempty"`
}

// RenderedComment is a comment with every piece of free text moved behind a
// marker.
type RenderedComment struct {
	ID          string          `json:"id"`
	Text        *UntrustedText  `json:"text,omitempty"`
	CreatedTime *string         `json:"createdTime,omitempty"`
	Author      *RenderedAuthor `json:"author,omitempty"`
	LikeCount   *int            `json:"likeCount,omitempty"`
	ReplyCount  *int            `json:"replyCount,omitempty"`
	Hidden      *bool           `json:"hidden,omitempty"`
	CanComment  *bool           `json:"canComment,omitempty"`
	CanHide     *bool           `json:"canHide,omitempty"`
	Permalink   *string         `json:"permalink,omitempty"`
	ParentID    *string         `json:"parentId,omitempty"`
}

// Untrusted wraps free text in its marker, truncating at a declared limit.
//
// Truncation counts RUNES, not bytes. Slicing a UTF-8 string at a byte offset
// can cut a multi-byte character in half and emit a replacement character, so a
// comment in any non-Latin script would come back corrupted at exactly the
// limit.
func Untrusted(value string, limit int) UntrustedText {
	runes := []rune(value)
	if len(runes) <= limit {
		return UntrustedText{Untrusted: true, Value: value, Truncated: false}
	}
	return UntrustedText{Untrusted: true, Value: string(runes[:limit]), Truncated: true}
}

// TextBudget is a drawdown allowance shared across everything rendered into one
// response.
type TextBudget struct {
	remaining int
}

func NewTextBudget(total int) *TextBudget {
	if total < 0 {
		total = 0
	}
	return &TextBudget{remaining: total}
}

// Remaining reports the characters still unspent.
func (b *TextBudget) Remaining() int { return b.remaining }

// Take spends cost if the whole amount is available.
//
// All-or-nothing on purpose: half a comment body is not a useful thing to
// return, and a partially-spent thread would have to be described as both
// complete and abbreviated.
func (b *TextBudget) Take(cost int) bool {
	if cost > b.remaining {
		return false
	}
	b.remaining -= cost
	return true
}

// CommentTextCost is what RenderComment would spend on this comment.
func CommentTextCost(c graph.Comment) int {
	cost := 0
	if c.Message != nil {
		cost += min(len([]rune(*c.Message)), MaxCommentChars)
	}
	if c.Author != nil && c.Author.Name != nil {
		cost += min(len([]rune(*c.Author.Name)), MaxAuthorNameChars)
	}
	return cost
}

// RenderComment moves every piece of comment free text into labelled, delimited
// fields.
//
// Comment text and author names are written by the public, and this server also
// holds tools that publish and hide. Structural separation does not stop a model
// being persuaded by content it legitimately reads — that non-defence is
// documented in the README — but it stops the text being mistaken for structure.
func RenderComment(c graph.Comment) RenderedComment {
	rendered := RenderedComment{
		ID:          c.ID,
		CreatedTime: c.CreatedTime,
		LikeCount:   c.LikeCount,
		ReplyCount:  c.ReplyCount,
		Hidden:      c.Hidden,
		CanComment:  c.CanComment,
		CanHide:     c.CanHide,
		Permalink:   c.Permalink,
		ParentID:    c.ParentID,
	}

	if c.Message != nil {
		text := Untrusted(*c.Message, MaxCommentChars)
		rendered.Text = &text
	}
	if c.Author != nil {
		author := &RenderedAuthor{ID: c.Author.ID}
		if c.Author.Name != nil {
			name := Untrusted(*c.Author.Name, MaxAuthorNameChars)
			author.Name = &name
		}
		rendered.Author = author
	}
	return rendered
}

// AbbreviateComment is the same comment with all of its free text withheld.
//
// Used once a response has spent its text budget: the caller still gets the id,
// status, counts, timestamps and author id — enough to count, group and act on
// the thread, or to fetch it specifically — without the body.
func AbbreviateComment(c graph.Comment) RenderedComment {
	rendered := RenderComment(c)
	rendered.Text = nil
	if rendered.Author != nil {
		rendered.Author = &RenderedAuthor{ID: rendered.Author.ID}
		if rendered.Author.ID == nil {
			// An author that carried only a name has nothing left to say once the
			// name is withheld.
			rendered.Author = nil
		}
	}
	return rendered
}
