package graph

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// "This ad has no Page post behind it" is an answer the product reports out
// loud. An empty string would be indistinguishable from it, and an empty answer
// reads as "no comments" when it means "not visible here".
func TestCreativeWithNoStoryIDReturnsNilNotEmpty(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"creative":{"id":"cr1"}}`))
	})

	client := NewMarketingClient(newTestTransport(t, server.URL))
	got, err := client.CreativeForAd(context.Background(), "ad1")
	if err != nil {
		t.Fatalf("CreativeForAd: %v", err)
	}
	if got.EffectiveObjectStoryID != nil {
		t.Fatalf("EffectiveObjectStoryID = %v, want nil", got.EffectiveObjectStoryID)
	}
	if got.ObjectStoryID != nil {
		t.Fatalf("ObjectStoryID = %v, want nil", got.ObjectStoryID)
	}
}

// effective_object_story_id resolves to the real post even when the creative was
// defined inline; object_story_id is the explicit reference.
func TestCreativeCarriesBothStoryIDs(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"creative":{"id":"cr1","effective_object_story_id":"pg1_p1","object_story_id":"pg1_p0"}}`))
	})

	client := NewMarketingClient(newTestTransport(t, server.URL))
	got, err := client.CreativeForAd(context.Background(), "ad1")
	if err != nil {
		t.Fatalf("CreativeForAd: %v", err)
	}
	if got.EffectiveObjectStoryID == nil || *got.EffectiveObjectStoryID != "pg1_p1" {
		t.Fatalf("EffectiveObjectStoryID = %v", got.EffectiveObjectStoryID)
	}
	if got.ObjectStoryID == nil || *got.ObjectStoryID != "pg1_p0" {
		t.Fatalf("ObjectStoryID = %v", got.ObjectStoryID)
	}

	// The creative is read as a nested selection on the ad, not as its own edge.
	if fields := server.queryFor(t, "/ad1").Get("fields"); !strings.HasPrefix(fields, "creative{") {
		t.Errorf("fields = %q, want a creative{...} selection", fields)
	}
}

// An ad with no creative at all is not a crash.
func TestAdWithNoCreativeIsNotAnError(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ad1"}`))
	})

	client := NewMarketingClient(newTestTransport(t, server.URL))
	got, err := client.CreativeForAd(context.Background(), "ad1")
	if err != nil {
		t.Fatalf("CreativeForAd: %v", err)
	}
	if got.EffectiveObjectStoryID != nil || got.ObjectStoryID != nil {
		t.Fatalf("creative = %+v, want no story ids", got)
	}
}

// status prefers effective_status, which accounts for a parent being paused or
// an account disabled; status alone reports only what was set on the campaign.
func TestCampaignsSurfaceBothStatuses(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"c1","name":"Spring","status":"ACTIVE","effective_status":"CAMPAIGN_PAUSED"}]}`))
	})

	client := NewMarketingClient(newTestTransport(t, server.URL))
	got, _, err := client.Campaigns(context.Background(), "act_1", PageOptions{})
	if err != nil {
		t.Fatalf("Campaigns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d campaigns", len(got))
	}
	if got[0].EffectiveStatus == nil || *got[0].EffectiveStatus != "CAMPAIGN_PAUSED" {
		t.Fatalf("EffectiveStatus = %v", got[0].EffectiveStatus)
	}
	if got[0].Status == nil || *got[0].Status != "ACTIVE" {
		t.Fatalf("Status = %v", got[0].Status)
	}
}

// Ad account ids are prefixed act_. A caller who has the bare number from Ads
// Manager must not get a 404 for it.
func TestAdAccountIDsAreNormalised(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	client := NewMarketingClient(newTestTransport(t, server.URL))
	ctx := context.Background()
	if _, _, err := client.Campaigns(ctx, "123456", PageOptions{}); err != nil {
		t.Fatalf("Campaigns: %v", err)
	}
	if _, _, err := client.Campaigns(ctx, "act_123456", PageOptions{}); err != nil {
		t.Fatalf("Campaigns: %v", err)
	}

	for _, u := range server.requests {
		if !strings.Contains(u.Path, "/act_123456/campaigns") {
			t.Fatalf("request path %q did not normalise the account id", u.Path)
		}
	}
}

// Ad sets and ads are read for their ids alone — the sweep fans out through them
// and needs nothing else.
func TestAdSetsAndAdsReturnIDs(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/adsets") {
			_, _ = w.Write([]byte(`{"data":[{"id":"as1"},{"id":"as2"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"ad1"}]}`))
	})

	client := NewMarketingClient(newTestTransport(t, server.URL))
	ctx := context.Background()

	adSets, _, err := client.AdSets(ctx, "c1", PageOptions{})
	if err != nil {
		t.Fatalf("AdSets: %v", err)
	}
	if len(adSets) != 2 || adSets[0] != "as1" {
		t.Fatalf("adSets = %v", adSets)
	}

	ads, _, err := client.Ads(ctx, "as1", PageOptions{})
	if err != nil {
		t.Fatalf("Ads: %v", err)
	}
	if len(ads) != 1 || ads[0] != "ad1" {
		t.Fatalf("ads = %v", ads)
	}
}
