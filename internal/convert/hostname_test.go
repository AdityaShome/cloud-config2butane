package convert

import "testing"

func TestHostnameExplicit(t *testing.T) {
	f := Hostname("node1", "")
	if f == nil || *f.Contents.Inline != "node1\n" {
		t.Fatalf("got %v", f)
	}
	if f.Path != hostnamePath {
		t.Errorf("got path %q", f.Path)
	}
}

func TestHostnameDerivedFromFQDN(t *testing.T) {
	f := Hostname("", "node1.cluster.example.com")
	if f == nil || *f.Contents.Inline != "node1\n" {
		t.Fatalf("got %v, want short hostname derived from fqdn", f)
	}
}

func TestHostnameExplicitWinsOverFQDN(t *testing.T) {
	f := Hostname("node1", "other.example.com")
	if f == nil || *f.Contents.Inline != "node1\n" {
		t.Fatalf("got %v, want explicit hostname to win", f)
	}
}

func TestHostnameNeitherSet(t *testing.T) {
	if f := Hostname("", ""); f != nil {
		t.Fatalf("expected nil, got %v", f)
	}
}

func TestHostnameFQDNWithoutDot(t *testing.T) {
	f := Hostname("", "node1")
	if f == nil || *f.Contents.Inline != "node1\n" {
		t.Fatalf("got %v", f)
	}
}
