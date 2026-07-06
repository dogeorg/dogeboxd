package core

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

type versionComparator struct {
	operator string
	version  string
}

var versionConstraintOperators = []string{">=", "<=", ">", "<", "="}

func VersionConstraintCheck(current string, constraint string) (bool, error) {
	if err := ValidateSemverVersion(current, "current"); err != nil {
		return false, err
	}
	comparators, err := parseVersionConstraint(constraint)
	if err != nil {
		return false, err
	}

	for _, comparator := range comparators {
		matches, err := versionComparatorMatches(current, comparator)
		if err != nil {
			return false, err
		}
		if !matches {
			return false, nil
		}
	}

	return true, nil
}

func ValidateSemverVersion(version string, label string) error {
	if !semver.IsValid(version) {
		return fmt.Errorf("invalid %s version %q", label, version)
	}

	return nil
}

func parseVersionConstraint(constraint string) ([]versionComparator, error) {
	trimmedConstraint := strings.TrimSpace(constraint)
	if trimmedConstraint == "" {
		return nil, fmt.Errorf("version constraint cannot be empty")
	}

	tokens := strings.Fields(trimmedConstraint)
	comparators := make([]versionComparator, 0, len(tokens))
	for _, token := range tokens {
		comparator, err := parseVersionComparator(token)
		if err != nil {
			return nil, err
		}
		comparators = append(comparators, comparator)
	}

	return comparators, nil
}

func parseVersionComparator(token string) (versionComparator, error) {
	for _, operator := range versionConstraintOperators {
		if strings.HasPrefix(token, operator) {
			version := strings.TrimSpace(strings.TrimPrefix(token, operator))
			if err := ValidateSemverVersion(version, "constraint"); err != nil {
				return versionComparator{}, err
			}

			return versionComparator{
				operator: operator,
				version:  version,
			}, nil
		}
	}

	return versionComparator{}, fmt.Errorf("invalid version comparator %q", token)
}

func versionComparatorMatches(current string, comparator versionComparator) (bool, error) {
	comparison := semver.Compare(current, comparator.version)

	switch comparator.operator {
	case ">":
		return comparison > 0, nil
	case ">=":
		return comparison >= 0, nil
	case "<":
		return comparison < 0, nil
	case "<=":
		return comparison <= 0, nil
	case "=":
		return comparison == 0, nil
	default:
		return false, fmt.Errorf("invalid version comparator operator %q", comparator.operator)
	}
}
