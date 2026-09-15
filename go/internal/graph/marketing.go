package graph

import (
	"context"
	"encoding/json"
	"strings"
)

// The Marketing surface, trimmed to what this server actually needs.
//
// No insights: this is a comment-triage server, and reading spend would be
// requesting data the capability has no use for on a token that may not carry
// the scope. The ad path exists for one reason — resolving an ad to the Page
// post behind it, which is the only route to comments on unpublished posts.

// AdAccount is an ad account this identity can reach.
type AdAccount struct {
	ID            string  `json:"id"`
	Name          *string `json:"name"`
	AccountStatus *int    `json:"account_status"`
	Currency      *string `json:"currency"`
	TimezoneName  *string `json:"timezone_name"`
	CreatedTime   *string `json:"created_time"`
	AmountSpent   *string `json:"amount_spent"`
	SpendCap      *string `json:"spend_cap"`
	BusinessID    *string `json:"business_id"`
}

// Campaign is a campaign, named so a person can choose one without knowing its
// id.
type Campaign struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
	// Status is what was set on the campaign itself.
	Status *string `json:"status"`
	// EffectiveStatus accounts for a parent being paused or an account being
	// disabled. Callers presenting one status to a person prefer this.
	EffectiveStatus *string `json:"effective_status"`
	Objective       *string `json:"objective"`
	DailyBudget     *string `json:"daily_budget"`
	LifetimeBudget  *string `json:"lifetime_budget"`
	StartTime       *string `json:"start_time"`
	StopTime        *string `json:"stop_time"`
	CreatedTime     *string `json:"created_time"`
	UpdatedTime     *string `json:"updated_time"`
}

// AdCreative is the creative behind an ad, read only for the post it points at.
//
// Both story ids are pointers because "this ad has no Page post behind it" is a
// real answer the product reports. An empty string would be indistinguishable
// from it, and the resulting empty response would read as "no comments" when it
// means "not visible here".
type AdCreative struct {
	ID string `json:"id"`
	// EffectiveObjectStoryID resolves to the real post even when the creative
	// was defined inline.
	EffectiveObjectStoryID *string `json:"effective_object_story_id"`
	// ObjectStoryID is the explicit reference.
	ObjectStoryID *string `json:"object_story_id"`
}

// MarketingClient reads the ad hierarchy.
//
// Marketing reads always carry the USER token, never a Page token: ads belong to
// an ad account, not to a Page.
type MarketingClient struct {
	transport *Transport
}

func NewMarketingClient(t *Transport) *MarketingClient {
	return &MarketingClient{transport: t}
}

// normalizeAccountID accepts the bare number a person copies out of Ads Manager
// as well as the act_-prefixed form Graph uses.
func normalizeAccountID(id string) string {
	if strings.HasPrefix(id, "act_") {
		return id
	}
	return "act_" + id
}

// AdAccounts lists the ad accounts this identity can reach.
func (c *MarketingClient) AdAccounts(ctx context.Context, opts PageOptions) ([]AdAccount, bool, error) {
	return readMarketing[AdAccount](ctx, c, "/me/adaccounts", AccountFields, opts)
}

// Campaigns lists an account's campaigns.
//
// Every campaign is listed regardless of status: a finished campaign's comments
// are still comments, and which statuses matter is the caller's judgement rather
// than this server's.
func (c *MarketingClient) Campaigns(ctx context.Context, accountID string, opts PageOptions) ([]Campaign, bool, error) {
	return readMarketing[Campaign](ctx, c, "/"+normalizeAccountID(accountID)+"/campaigns", CampaignFields, opts)
}

// AdSets returns a campaign's ad set ids. The sweep fans out through them and
// needs nothing else.
func (c *MarketingClient) AdSets(ctx context.Context, campaignID string, opts PageOptions) ([]string, bool, error) {
	return readIDs(ctx, c, "/"+campaignID+"/adsets", AdSetFields, opts)
}

// Ads returns an ad set's ad ids.
func (c *MarketingClient) Ads(ctx context.Context, adSetID string, opts PageOptions) ([]string, bool, error) {
	return readIDs(ctx, c, "/"+adSetID+"/ads", AdFields, opts)
}

// CreativeForAd reads the creative as a nested selection on the ad rather than
// as its own edge — one request instead of two.
//
// An ad with no creative is not an error: it is an ad whose comments cannot be
// reached from the Page side, and the caller says so out loud.
func (c *MarketingClient) CreativeForAd(ctx context.Context, adID string) (AdCreative, error) {
	var body struct {
		ID       string      `json:"id"`
		Creative *AdCreative `json:"creative"`
	}
	err := c.transport.Get(ctx, "/"+adID, map[string]string{
		"fields": "creative{" + strings.Join(AdCreativeFields, ",") + "}",
	}, &body)
	if err != nil {
		return AdCreative{}, err
	}
	if body.Creative == nil {
		return AdCreative{ID: adID}, nil
	}
	return *body.Creative, nil
}

func readMarketing[T any](
	ctx context.Context,
	c *MarketingClient,
	path string,
	fields []string,
	opts PageOptions,
) ([]T, bool, error) {
	raws, truncated, err := c.transport.GetPage(ctx, path, PageOptions{
		MaxItems: opts.MaxItems,
		PageSize: opts.PageSize,
		Params:   map[string]string{"fields": strings.Join(fields, ",")},
	})
	if err != nil {
		return nil, false, err
	}

	out := make([]T, 0, len(raws))
	for _, message := range raws {
		var row T
		if err := json.Unmarshal(message, &row); err != nil {
			return nil, false, &MetaAPIError{Message: "Meta returned a row this client could not parse: " + err.Error()}
		}
		out = append(out, row)
	}
	return out, truncated, nil
}

func readIDs(
	ctx context.Context,
	c *MarketingClient,
	path string,
	fields []string,
	opts PageOptions,
) ([]string, bool, error) {
	rows, truncated, err := readMarketing[struct {
		ID string `json:"id"`
	}](ctx, c, path, fields, opts)
	if err != nil {
		return nil, false, err
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, truncated, nil
}
