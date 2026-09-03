package github

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// decodeListingPage validates one listing page body. A page larger than the
// documented size is malformed: OrgTop never truncates an oversized response to
// manufacture its own bound.
func decodeListingPage(organization string, body []byte) ([]listingRecord, error) {
	var payloads []listingPayload
	if err := json.Unmarshal(body, &payloads); err != nil {
		return nil, fmt.Errorf("%w: %s did not return a github organization listing page", ErrInvalidListing, organization)
	}
	if len(payloads) > pageSize {
		return nil, fmt.Errorf("%w: a %s listing page returned %d records, at most %d are valid",
			ErrInvalidListing, organization, len(payloads), pageSize)
	}
	records := make([]listingRecord, 0, len(payloads))
	for index, payload := range payloads {
		record, err := normalizeListingRecord(organization, index, payload)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// normalizeListingRecord validates the required facts of one listed repository.
// Canonical equality, not bytewise casing equality, validates the repeated
// fields, and the first valid full_name spelling is the retained one.
func normalizeListingRecord(organization string, index int, payload listingPayload) (listingRecord, error) {
	invalid := func(reason string) (listingRecord, error) {
		return listingRecord{}, fmt.Errorf("%w: record %d of the %s listing %s", ErrInvalidListing, index, organization, reason)
	}
	if payload.Owner == nil || payload.Archived == nil || payload.Disabled == nil || payload.Fork == nil {
		return invalid("omits a required field")
	}
	repository, err := domain.ParseRepository(payload.FullName)
	if err != nil {
		return invalid("has no lexically valid full_name")
	}
	repeated, err := domain.ParseRepository(payload.Owner.Login + "/" + payload.Name)
	if err != nil || repeated.Key() != repository.Key() {
		return invalid("does not repeat its own owner and name")
	}
	if !strings.EqualFold(repository.Owner(), organization) {
		return invalid("belongs to another owner")
	}
	if payload.Owner.Type != organizationOwnerType {
		return invalid("is not owned by an organization")
	}
	return listingRecord{
		repository: repository,
		archived:   *payload.Archived,
		disabled:   *payload.Disabled,
		fork:       *payload.Fork,
	}, nil
}
