package main

import "testing"

func testCodeStorePaths(t *testing.T, dbPath string) (string, string) {
	t.Helper()
	a, b, err := getCodeStorePaths(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}
func testFindingsStorePath(t *testing.T, dbPath string) string {
	t.Helper()
	p, err := getFindingsStorePath(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func testSurveyStorePath(t *testing.T, dbPath string) string {
	t.Helper()
	p, err := getSurveyStorePath(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
