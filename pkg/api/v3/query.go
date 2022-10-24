package api

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type Query interface {
	encoding.UnionValue
	QueryType() QueryType
	IsValid() error
}

func (q *ChainQuery) IsValid() error {
	err := q.baseIsValid()
	if err != nil {
		return err
	}

	hasName := q.Name != ""
	hasIndex := q.Index != nil
	hasEntry := q.Entry != nil
	hasRange := q.Range != nil

	if hasRange && (hasIndex || hasEntry) {
		return errors.Format(errors.StatusBadRequest, "range is mutually exclusive with index and entry")
	}

	if !hasName && (hasIndex || hasEntry || hasRange) {
		return errors.Format(errors.StatusBadRequest, "name is required when querying by index, entry, or range")
	}

	return nil
}

func (q *DataQuery) IsValid() error {
	err := q.baseIsValid()
	if err != nil {
		return err
	}

	hasIndex := q.Index != nil
	hasEntry := q.Entry != nil
	hasRange := q.Range != nil

	if hasRange && (hasIndex || hasEntry) {
		return errors.Format(errors.StatusBadRequest, "range is mutually exclusive with index and entry")
	}

	return nil
}

func (q *BlockQuery) IsValid() error {
	err := q.baseIsValid()
	if err != nil {
		return err
	}

	hasMinorIndex := q.Minor != nil      // A
	hasMajorIndex := q.Major != nil      // B
	hasMinorRange := q.MinorRange != nil // C
	hasMajorRange := q.MajorRange != nil // D
	hasEntryRange := q.EntryRange != nil // E

	// (E) AB \ CD
	// 0  00 01 11 10   1  00 01 11 10
	// 00  0  1  0  1   00  0  0  0  0
	// 01  1  0  0  1   01  0  0  0  0
	// 11  0  0  0  0   11  0  0  0  0
	// 10  1  0  0  0   10  1  0  0  0

	if !hasMinorIndex && !hasMajorIndex && !hasMinorRange && !hasMajorRange {
		return errors.Format(errors.StatusBadRequest, "nothing to do: minor, major, minor range, and major range are unspecified")
	}
	if hasMinorIndex && hasMajorIndex {
		return errors.Format(errors.StatusBadRequest, "minor and major are mutually exclusive")
	}
	if hasMinorRange && hasMajorRange {
		return errors.Format(errors.StatusBadRequest, "minor range and major range are mutually exclusive")
	}
	if hasMinorIndex && (hasMinorRange || hasMajorRange) {
		return errors.Format(errors.StatusBadRequest, "minor is mutually exclusive with minor range and major range")
	}
	if hasMajorIndex && hasMajorRange {
		return errors.Format(errors.StatusBadRequest, "major and major range are mutually exclusive")
	}
	if hasEntryRange && (hasMajorIndex || hasMinorRange || hasMajorRange) {
		return errors.Format(errors.StatusBadRequest, "entry range is mutually exclusive with major, minor range, and major range")
	}

	return nil
}
