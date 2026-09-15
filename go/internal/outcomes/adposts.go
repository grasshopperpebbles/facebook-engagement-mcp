package outcomes

import (
	"context"
	"fmt"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

// AdTarget names what to resolve. Exactly one field is set.
type AdTarget struct {
	Ad       *string
	Campaign *string
}

// ResolveAdPosts resolves an ad or campaign to the Page posts behind it.
//
// Ad -> creative -> effective_object_story_id -> Page post. This is NOT a
// convenience over the page sweep — it is the only route to ad comments. /feed
// does not return unpublished posts (verified live 2026-09-03, against Meta's
// documentation saying it does) and ads usually run on them, so a post reached
// this way is one no Page-level sweep would have found.
//
// Returns post ids in a deterministic order. Several ads commonly share one
// post, so this de-duplicates — but through a slice and a seen-set rather than
// by ranging a map, because ranging a Go map is randomised and the response
// would reorder itself run to run.
func ResolveAdPosts(
	ctx context.Context,
	marketing *graph.MarketingClient,
	target AdTarget,
) ([]string, []string, error) {
	notes := []string{}

	adIDs, err := adIDsFor(ctx, marketing, target)
	if err != nil {
		return nil, nil, err
	}

	postIDs := []string{}
	seen := map[string]bool{}

	for _, adID := range adIDs {
		creative, err := marketing.CreativeForAd(ctx, adID)
		if err != nil {
			return nil, nil, err
		}

		postID := creative.EffectiveObjectStoryID
		if postID == nil {
			postID = creative.ObjectStoryID
		}
		if postID == nil || *postID == "" {
			// Some formats never produce a Page post, so their comments are
			// unreachable from the Page side. An empty answer would look like
			// "no comments" rather than "not visible here" — which is the failure
			// mode the whole capability exists to prevent.
			notes = append(notes, fmt.Sprintf(
				"Ad %s has no Page post behind it, so its comments cannot be read from the Page. "+
					"Some ad formats do not create a Page post object.", adID))
			continue
		}

		if !seen[*postID] {
			seen[*postID] = true
			postIDs = append(postIDs, *postID)
		}
	}

	return postIDs, notes, nil
}

// adIDsFor expands the target into the ads to resolve. A campaign fans out
// through its ad sets.
func adIDsFor(
	ctx context.Context,
	marketing *graph.MarketingClient,
	target AdTarget,
) ([]string, error) {
	if target.Ad != nil && *target.Ad != "" {
		return []string{*target.Ad}, nil
	}
	if target.Campaign == nil || *target.Campaign == "" {
		return nil, fmt.Errorf("give an ad or campaign to resolve")
	}

	adSets, _, err := marketing.AdSets(ctx, *target.Campaign, graph.PageOptions{MaxItems: 100})
	if err != nil {
		return nil, err
	}

	collected := []string{}
	for _, adSetID := range adSets {
		ads, _, err := marketing.Ads(ctx, adSetID, graph.PageOptions{MaxItems: 100})
		if err != nil {
			return nil, err
		}
		collected = append(collected, ads...)
	}
	return collected, nil
}
