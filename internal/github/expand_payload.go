package github

// listingPayload is one entry of a GET /orgs/{organization}/repos page. Every
// modelled field is required by RG-010, so the booleans and the owner are
// pointers: a missing field is malformed rather than silently false.
type listingPayload struct {
	FullName string               `json:"full_name"`
	Name     string               `json:"name"`
	Owner    *listingOwnerPayload `json:"owner"`
	Archived *bool                `json:"archived"`
	Disabled *bool                `json:"disabled"`
	Fork     *bool                `json:"fork"`
}

// listingOwnerPayload is the owner a listed repository belongs to. Expansion
// admits an organization owner only.
type listingOwnerPayload struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}
