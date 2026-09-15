package graph

// Raw Graph shapes to the shapes the rest of this server speaks.
//
// Every conversion here preserves absence. A pointer that arrives nil leaves
// nil; it is never replaced with a zero value, because downstream the two mean
// different things. The single exception is PagePost.IsPublished, and it is
// commented where it happens.

func NormalizePage(raw RawPage) Page {
	return Page{
		ID:       raw.ID,
		Name:     raw.Name,
		Category: raw.Category,
		Tasks:    raw.Tasks,
	}
}

func NormalizePost(raw RawPost) PagePost {
	// The one defaulting field. Graph omits is_published on some edges, and an
	// omitted value must not be read as "this is an ad post" — that would label
	// ordinary organic posts as ad-backed throughout the response.
	published := true
	if raw.IsPublished != nil {
		published = *raw.IsPublished
	}

	return PagePost{
		ID:          raw.ID,
		Message:     raw.Message,
		CreatedTime: raw.CreatedTime,
		Permalink:   raw.PermalinkURL,
		IsPublished: published,
	}
}

func NormalizePhoto(raw RawPhoto) PagePhoto {
	return PagePhoto{
		ID:          raw.ID,
		CreatedTime: raw.CreatedTime,
		PostID:      raw.PageStoryID,
	}
}

func NormalizeComment(raw RawComment) Comment {
	comment := Comment{
		ID:          raw.ID,
		Message:     raw.Message,
		CreatedTime: raw.CreatedTime,
		LikeCount:   raw.LikeCount,
		ReplyCount:  raw.CommentCount,
		Hidden:      raw.IsHidden,
		CanComment:  raw.CanComment,
		CanHide:     raw.CanHide,
		Permalink:   raw.PermalinkURL,
	}

	// `from` present with no id is not an identity, but it IS a returned field.
	// Keep the distinction rather than collapsing it: nil means Graph said
	// nothing, and a non-nil Author with a nil ID means Graph said something
	// that does not identify anyone.
	if raw.From != nil {
		comment.Author = &Author{ID: raw.From.ID, Name: raw.From.Name}
	}

	// A parent object with no id does not invent one.
	if raw.Parent != nil && raw.Parent.ID != nil {
		comment.ParentID = raw.Parent.ID
	}

	return comment
}
