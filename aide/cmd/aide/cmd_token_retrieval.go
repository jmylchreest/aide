package main

import "fmt"

func formatRetrievalEvidence(attrs map[string]string) string {
	labels := map[string]string{
		"full_file":          "full-file text matched",
		"range":              "source range matched",
		"referenced":         "source receipt matched",
		"unverified":         "source delivery unverified",
		"failed":             "retrieval failed",
		"pending":            "retrieval still running",
		"search":             "search observed; file delivery unverified",
		"unclassified_shell": "shell retrieval coverage unknown",
	}
	label := labels[attrs["retrieval_status"]]
	if label == "" {
		if attrs["source_references"] == "" {
			return ""
		}
		label = "server source reference; host delivery unknown"
	}
	text := "  Retrieval: " + label
	if method := attrs["retrieval_method"]; method != "" {
		text += "; method=" + method
	}
	if attrs["source_verification"] == "current_range_match" {
		text += fmt.Sprintf("; lines %s-%s matched; undisplayed source version unverified", attrs["delivered_start_line"], attrs["delivered_end_line"])
	}
	if attrs["source_references"] != "" {
		text += "; conditional references in --json (not avoided reads)"
	}
	return text + "\n"
}
