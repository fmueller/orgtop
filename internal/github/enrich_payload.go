package github

// Transport payload types for the RG-003 enrichment endpoints. Like the events
// payload they are private to the adapter: no exported signature mentions them,
// so no pagination, header, or JSON type reaches the domain.

// commitResponse is one page of GET /repos/{owner}/{repo}/commits/{sha}.
type commitResponse struct {
	SHA     string        `json:"sha"`
	Parents []refPayload  `json:"parents"`
	Files   []filePayload `json:"files"`
}

// compareResponse is GET /repos/{owner}/{repo}/compare/{base}...{head}.
type compareResponse struct {
	URL        string        `json:"url"`
	BaseCommit *refPayload   `json:"base_commit"`
	Files      []filePayload `json:"files"`
}

// pullRequestResponse is GET /repos/{owner}/{repo}/pulls/{number}.
type pullRequestResponse struct {
	URL    string          `json:"url"`
	Number int             `json:"number"`
	Base   *pullEndPayload `json:"base"`
	Head   *pullEndPayload `json:"head"`
}

// pullEndPayload is one end of a pull request. Only the immutable object and the
// repository identity that binds it are modelled.
type pullEndPayload struct {
	SHA        string             `json:"sha"`
	Repository *repositoryPayload `json:"repo"`
}

// repositoryPayload carries the canonical owner/repository identity of a
// pull request end.
type repositoryPayload struct {
	FullName string `json:"full_name"`
}

// filePayload is one changed-file record. A rename carries both names; every
// other documented status contributes its filename.
type filePayload struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"`
	PreviousFilename string `json:"previous_filename"`
}
