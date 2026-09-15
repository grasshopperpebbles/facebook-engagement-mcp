package graph

import (
	"context"
	"encoding/json"
	"strings"
)

// PagesClient reads Facebook Page engagement: Pages, their posts, and comments.
//
// Reads /{page-id}/feed. That edge was chosen because Meta's documentation
// states /feed returns unpublished posts where /posts and /published_posts do
// not — unpublished posts being the object type behind ads.
//
// THAT IS NOT WHAT HAPPENS. Verified against a live Page on 2026-09-03: an
// unpublished post (is_published: false, confirmed by fetching it directly) was
// returned by NONE of /feed, /posts or /published_posts, with or without
// is_published=false or include_hidden=true. The two edges returned
// byte-identical id lists. /promotable_posts does not exist.
//
// So no Page-level sweep discovers unpublished posts. Their comments ARE
// readable — the /comments edge on such a post answers normally — but only if
// you already have the post id, which in practice means resolving an ad to its
// creative's effective_object_story_id. /feed is kept over /posts because it is
// a superset in principle (it can include visitor posts), not because it finds
// dark posts. It does not.
type PagesClient struct {
	transport *Transport
}

func NewPagesClient(t *Transport) *PagesClient {
	return &PagesClient{transport: t}
}

// WithTransport returns a client bound to a different token, for per-Page reads.
func (c *PagesClient) WithTransport(t *Transport) *PagesClient {
	return &PagesClient{transport: t}
}

// Transport exposes the underlying transport for callers that need to write.
func (c *PagesClient) Transport() *Transport { return c.transport }

// readPage is the shared shape of every list read here: request a field
// selection, page it, and normalize each row.
func readPage[Raw any, Out any](
	ctx context.Context,
	c *PagesClient,
	path string,
	fields []string,
	normalize func(Raw) Out,
	opts PageOptions,
	extra map[string]string,
) ([]Out, bool, error) {
	params := map[string]string{"fields": strings.Join(fields, ",")}
	for key, value := range extra {
		params[key] = value
	}
	for key, value := range opts.Params {
		params[key] = value
	}

	raws, truncated, err := c.transport.GetPage(ctx, path, PageOptions{
		MaxItems: opts.MaxItems,
		PageSize: opts.PageSize,
		Params:   params,
	})
	if err != nil {
		return nil, false, err
	}

	out := make([]Out, 0, len(raws))
	for _, message := range raws {
		var raw Raw
		if err := json.Unmarshal(message, &raw); err != nil {
			return nil, false, &MetaAPIError{Message: "Meta returned a row this client could not parse: " + err.Error()}
		}
		out = append(out, normalize(raw))
	}
	return out, truncated, nil
}

// List returns the Pages this identity can reach. It requests PageFields and
// never access_token — only Credentials asks for credential material.
func (c *PagesClient) List(ctx context.Context, opts PageOptions) ([]Page, bool, error) {
	return readPage(ctx, c, "/me/accounts", PageFields, NormalizePage, opts, nil)
}

// Credentials returns Page access tokens. Credential material — never log it,
// return it through a tool, or cache it to disk.
//
// Both outcomes are reported. A row Graph listed without an access_token used to
// be filtered away, which left the caller unable to tell it from a Page Graph
// never listed — and the caller then guessed, naming administration or
// pages_show_list for a case that may be neither.
func (c *PagesClient) Credentials(ctx context.Context) (PageCredentials, error) {
	raws, _, err := c.transport.GetPage(ctx, "/me/accounts", PageOptions{
		Params: map[string]string{"fields": strings.Join(PageCredentialFields, ",")},
	})
	if err != nil {
		return PageCredentials{}, err
	}

	result := PageCredentials{Usable: []PageCredential{}, Tokenless: []string{}}
	for _, message := range raws {
		var raw RawPage
		if err := json.Unmarshal(message, &raw); err != nil {
			return PageCredentials{}, &MetaAPIError{Message: "Meta returned a Page row this client could not parse: " + err.Error()}
		}
		if raw.AccessToken == nil || *raw.AccessToken == "" {
			result.Tokenless = append(result.Tokenless, raw.ID)
			continue
		}
		// Tasks is passed through rather than defaulted: an absent field is not
		// an empty role list.
		result.Usable = append(result.Usable, PageCredential{
			PageID:      raw.ID,
			AccessToken: *raw.AccessToken,
			Tasks:       raw.Tasks,
		})
	}
	return result, nil
}

// PostListOptions adds the server-side feed filter to a paged read.
type PostListOptions struct {
	PageOptions
	// Since is an ISO date or Unix timestamp. Graph filters the feed server-side.
	Since string
}

// Posts returns a Page's feed.
func (c *PagesClient) Posts(ctx context.Context, pageID string, opts PostListOptions) ([]PagePost, bool, error) {
	extra := map[string]string{}
	if opts.Since != "" {
		extra["since"] = opts.Since
	}
	return readPage(ctx, c, "/"+pageID+"/feed", PostFields, NormalizePost, opts.PageOptions, extra)
}

// Photos returns photos uploaded to a Page, carrying the post each belongs to.
//
// Not for display — PhotoFields requests no image at all. This exists so a
// caller can compare the posts a photo names against the posts the feed
// returned, because those two sets differ: a post published through one app is
// not readable by another app's Page token, and the photos inside it stay
// reachable while the post does not (T-37, confirmed 2026-09-15 against two
// tokens on one Page).
//
// type=uploaded is the Page's own photos rather than ones it is tagged in. A
// photo somebody else posted names somebody else's story, which would be counted
// as a post this token cannot see and would be right for the wrong reason.
func (c *PagesClient) Photos(ctx context.Context, pageID string, opts PageOptions) ([]PagePhoto, bool, error) {
	return readPage(ctx, c, "/"+pageID+"/photos", PhotoFields, NormalizePhoto, opts,
		map[string]string{"type": "uploaded"})
}

// CommentsForPost returns a post's TOP-LEVEL comments.
//
// filter=toplevel because replies are fetched deliberately, per comment, so the
// sweep knows which level it is reading. order=chronological because each
// individual call is then in time order — which is what makes the concatenation
// in the sweep the thing that destroys ordering, rather than the calls.
func (c *PagesClient) CommentsForPost(ctx context.Context, postID string, opts PageOptions) ([]Comment, bool, error) {
	return readPage(ctx, c, "/"+postID+"/comments", CommentFields, NormalizeComment, opts,
		map[string]string{"filter": "toplevel", "order": "chronological"})
}

// Replies returns the replies to one comment. No toplevel filter here — that is
// the whole point of the edge.
func (c *PagesClient) Replies(ctx context.Context, commentID string, opts PageOptions) ([]Comment, bool, error) {
	return readPage(ctx, c, "/"+commentID+"/comments", CommentFields, NormalizeComment, opts,
		map[string]string{"order": "chronological"})
}

// Comment reads one comment by id.
func (c *PagesClient) Comment(ctx context.Context, commentID string) (Comment, error) {
	var raw RawComment
	err := c.transport.Get(ctx, "/"+commentID, map[string]string{
		"fields": strings.Join(CommentFields, ","),
	}, &raw)
	if err != nil {
		return Comment{}, err
	}
	return NormalizeComment(raw), nil
}

// Reply publishes a reply to a comment, as whatever identity the transport
// carries. A write: never retried, and public the moment it exists.
func (c *PagesClient) Reply(ctx context.Context, commentID, message string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.transport.Post(ctx, "/"+commentID+"/comments", map[string]string{"message": message}, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// SetHidden hides or unhides a comment. Hide and unhide are one endpoint and one
// boolean, which is why this is one method rather than two.
func (c *PagesClient) SetHidden(ctx context.Context, commentID string, hidden bool) error {
	value := "false"
	if hidden {
		value = "true"
	}
	var out struct {
		Success bool `json:"success"`
	}
	return c.transport.Post(ctx, "/"+commentID, map[string]string{"is_hidden": value}, &out)
}

// Identity reports what this token IS, as Meta sees it.
//
// Exists because the identity behind a token changes the answer: a Business
// System User token was shown three posts on a Page where a personally-granted
// token for the same app was shown five, and returned no author for comments the
// other identified (2026-09-15). Neither difference announces itself, so a
// caller that cannot name its own identity cannot explain a short answer.
//
// Never fails for an invalid token — an unusable token is an answer, and
// Valid: false is how it is returned.
//
// /debug_token takes the token under inspection as a QUERY PARAMETER and accepts
// it no other way, which is the one deliberate exception to this client's
// tokens-in-headers rule. Confined to graph.facebook.com over TLS, and the
// transport logs no URLs.
func (c *PagesClient) Identity(ctx context.Context) (TokenIdentity, error) {
	token, err := c.transport.currentToken(ctx)
	if err != nil {
		return TokenIdentity{Valid: false}, nil
	}

	var body struct {
		Data *struct {
			Type        *string `json:"type"`
			AppID       *string `json:"app_id"`
			Application *string `json:"application"`
			ExpiresAt   *int64  `json:"expires_at"`
			IsValid     *bool   `json:"is_valid"`
		} `json:"data"`
	}

	if err := c.transport.Get(ctx, "/debug_token", map[string]string{"input_token": token}, &body); err != nil {
		// A token Graph will not even debug is not a crash, it is a false.
		return TokenIdentity{Valid: false}, nil
	}
	if body.Data == nil {
		return TokenIdentity{Valid: false}, nil
	}

	// expires_at: 0 means never, and 0 is falsy in most languages — the
	// distinction this whole project turns on. A pointer keeps it.
	return TokenIdentity{
		Type:      body.Data.Type,
		AppID:     body.Data.AppID,
		AppName:   body.Data.Application,
		ExpiresAt: body.Data.ExpiresAt,
		Valid:     body.Data.IsValid != nil && *body.Data.IsValid,
	}, nil
}
