package graph

// Structural types for the Page resources this client reads.
//
// Not runtime validated, matching the Marketing surface: Meta adds fields
// freely, and validating responses would break on harmless additions.
//
// EVERY FIELD BUT id IS A POINTER, and in Go that is a correctness rule rather
// than a style one. Graph omits rather than nulls anything the token lacks
// permission to see, and Go has no undefined — a zero struct is a struct, and a
// false is a false. The product's only answer turns on telling an absent field
// from a zero one: statusBasis is author_identity or reply_count entirely on
// whether an author was present anywhere, so a value-typed Author would answer
// "yes" to data that has no authors at all. The same trap recurs wherever zero
// is meaningful — comment_count, is_hidden, can_hide.

// RawPage is a Page as Graph returns it.
type RawPage struct {
	ID       string    `json:"id"`
	Name     *string   `json:"name"`
	Category *string   `json:"category"`
	Tasks    *[]string `json:"tasks"`
	// AccessToken is credential material. It never leaves this client except
	// through PagesClient.Credentials.
	AccessToken *string `json:"access_token"`
}

// RawPost is a Page post as Graph returns it.
type RawPost struct {
	ID           string  `json:"id"`
	Message      *string `json:"message"`
	CreatedTime  *string `json:"created_time"`
	PermalinkURL *string `json:"permalink_url"`
	IsPublished  *bool   `json:"is_published"`
}

// RawPhoto is a photo on a Page, read only for the post it belongs to.
//
// page_story_id is the {page-id}_{post-id} of the post the photo was published
// in. It exists here for one purpose: a photo can be reachable when the post
// carrying it is not, and comparing the two is the only way this client can
// notice it is being shown less than the Page holds. See T-37.
type RawPhoto struct {
	ID          string  `json:"id"`
	CreatedTime *string `json:"created_time"`
	PageStoryID *string `json:"page_story_id"`
}

// RawAuthor is Graph's `from` object.
type RawAuthor struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
}

// RawComment is a comment as Graph returns it.
type RawComment struct {
	ID           string     `json:"id"`
	Message      *string    `json:"message"`
	CreatedTime  *string    `json:"created_time"`
	From         *RawAuthor `json:"from"`
	LikeCount    *int       `json:"like_count"`
	CommentCount *int       `json:"comment_count"`
	IsHidden     *bool      `json:"is_hidden"`
	CanComment   *bool      `json:"can_comment"`
	CanHide      *bool      `json:"can_hide"`
	PermalinkURL *string    `json:"permalink_url"`
	Parent       *struct {
		ID *string `json:"id"`
	} `json:"parent"`
}

// Page is a Page, normalized.
type Page struct {
	ID       string  `json:"id"`
	Name     *string `json:"name,omitempty"`
	Category *string `json:"category,omitempty"`
	// Tasks are the Page roles this token holds, e.g. MODERATE, ANALYZE.
	//
	// Absent and empty are different answers. nil means Graph did not return the
	// field, so what this identity may do is unknown; an empty slice means Graph
	// returned it empty, so the identity holds no role. A caller refusing a write
	// on a missing role must act on the second and not the first.
	Tasks *[]string `json:"tasks,omitempty"`
}

// PageCredential is a Page access token and the tasks it carries. Treat as a
// secret.
type PageCredential struct {
	PageID      string
	AccessToken string
	// Tasks carries the same absent-versus-empty distinction as Page.Tasks.
	Tasks *[]string
}

// PageCredentials is what /me/accounts yielded, with the two outcomes kept
// apart.
//
// A Page can appear in that listing and still carry no access_token. Folding
// those rows away made them indistinguishable from Pages the listing never
// mentioned, and the caller then reported the wrong cause for both.
type PageCredentials struct {
	// Usable are the Pages that yielded a token. Credential material.
	Usable []PageCredential
	// Tokenless are Page ids Graph listed and withheld a token for. Carries no
	// secret, and is the difference between "you cannot" and "we never saw it".
	Tokenless []string
}

// TokenIdentity says which identity a token speaks for.
//
// SYSTEM_USER — a Business Manager system user. Does not expire, and has been
// observed on 2026-09-15 being shown FEWER posts on a Page, and returning NO
// author for comments written by people, than a personally-granted token for the
// same app on the same Page. Both differences are silent.
// USER — a person granted this app access. Expires.
// PAGE — a Page token. There is no /me/accounts to exchange from one.
//
// Reported rather than acted on: which trade-off to take is the caller's, and an
// answer that quietly picked one would be the thing this type exists to stop.
type TokenIdentity struct {
	Type    *string
	AppID   *string
	AppName *string
	// ExpiresAt is Unix seconds; 0 means never. nil means Graph did not say.
	ExpiresAt *int64
	Valid     bool
}

// PagePhoto is a Page photo, normalized. PostID is the post it was published in.
type PagePhoto struct {
	ID          string  `json:"id"`
	CreatedTime *string `json:"createdTime,omitempty"`
	PostID      *string `json:"postId,omitempty"`
}

// PagePost is a Page post, normalized.
type PagePost struct {
	ID          string  `json:"id"`
	Message     *string `json:"message,omitempty"`
	CreatedTime *string `json:"createdTime,omitempty"`
	Permalink   *string `json:"permalink,omitempty"`
	// IsPublished is false for unpublished posts, which are the object type
	// behind ads.
	//
	// This is the ONE field that defaults rather than staying absent: it is true
	// when Graph omits the field, because an absent value must not be read as
	// "this is an ad post". ThreadPost carries a *bool for the opposite reason —
	// there, absent means the post node was never read.
	IsPublished bool `json:"isPublished"`
}

// Author is a comment's author, normalized.
type Author struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

// Comment is a comment, normalized.
//
// Author is absent, not empty, when Graph does not return `from`. Which tokens
// get `from` for which authors is NOT established — an earlier claim that Graph
// returns it for Pages and withholds it for people was overturned when a
// personally-granted token for the same app returned it for the same person's
// same comments (T-42). Consumers handle its absence rather than assume a cause.
type Comment struct {
	ID          string  `json:"id"`
	Message     *string `json:"message,omitempty"`
	CreatedTime *string `json:"createdTime,omitempty"`
	Author      *Author `json:"author,omitempty"`
	LikeCount   *int    `json:"likeCount,omitempty"`
	ReplyCount  *int    `json:"replyCount,omitempty"`
	Hidden      *bool   `json:"hidden,omitempty"`
	// CanComment is whether a reply can be posted to this comment.
	CanComment *bool   `json:"canComment,omitempty"`
	CanHide    *bool   `json:"canHide,omitempty"`
	Permalink  *string `json:"permalink,omitempty"`
	ParentID   *string `json:"parentId,omitempty"`
}
