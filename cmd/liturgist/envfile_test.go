// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	in := "\xef\xbb\xbf# comment\n\nLITURGIST_BASE_URL = \"https://x.example\"\r\nA='b c'\nEMPTY=\nQ=\"half\n"
	got, err := parseEnvFile(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"LITURGIST_BASE_URL": "https://x.example", "A": "b c", "EMPTY": "", "Q": `"half`}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseEnvFileRejectsBadLines(t *testing.T) {
	for _, in := range []string{"nokey\n", "=value\n", "ok=1\nbroken\n"} {
		if _, err := parseEnvFile(strings.NewReader(in)); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestApplyEnvKeepsTheEnvironment(t *testing.T) {
	env := map[string]string{"A": "env"}
	set := map[string]string{}
	err := applyEnv(map[string]string{"A": "file", "B": "file"},
		func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		func(k, v string) error { set[k] = v; return nil })
	if err != nil || len(set) != 1 || set["B"] != "file" {
		t.Fatalf("set %v, err %v", set, err)
	}
}

// fakeEnv is an environment for the settings tests.
type fakeEnv map[string]string

func (e fakeEnv) lookup(k string) (string, bool) { v, ok := e[k]; return v, ok }
func (e fakeEnv) set(k, v string) error          { e[k] = v; return nil }

func settingsDir(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, envFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TC-611: a command typed by hand uses the settings and the data folder of the service.
func TestLoadServiceSettings(t *testing.T) {
	dir := settingsDir(t, "LITURGIST_BASE_URL=https://liturgi.example.org\nLITURGIST_BACKUP_TIME=off\n")
	env := fakeEnv{"LITURGIST_BACKUP_TIME": "03:00"}
	var stderr bytes.Buffer
	if !loadServiceSettings([]string{"backup", "x.zip"}, &stderr, dir, env.lookup, env.set) {
		t.Fatal(stderr.String())
	}
	if env["LITURGIST_BASE_URL"] != "https://liturgi.example.org" {
		t.Errorf("the file was not read: %v", env)
	}
	if env["LITURGIST_BACKUP_TIME"] != "03:00" {
		t.Errorf("the environment must win over the file: %v", env)
	}
	if env["LITURGIST_DATA_DIR"] != filepath.Join(dir, "data") {
		t.Errorf("data folder: %v", env)
	}
}

func TestLoadServiceSettingsKeepsDataDir(t *testing.T) {
	dir := settingsDir(t, "LITURGIST_DATA_DIR=D:\\church\n")
	env := fakeEnv{"LITURGIST_DATA_DIR": "E:\\mine"}
	if !loadServiceSettings([]string{"user", "list"}, &bytes.Buffer{}, dir, env.lookup, env.set) {
		t.Fatal("failed")
	}
	if env["LITURGIST_DATA_DIR"] != "E:\\mine" {
		t.Errorf("an explicit data folder must stay: %v", env)
	}
}

// No file means no service: nothing is set, not even the data folder.
func TestLoadServiceSettingsWithoutFile(t *testing.T) {
	env := fakeEnv{}
	if !loadServiceSettings([]string{"backup"}, &bytes.Buffer{}, t.TempDir(), env.lookup, env.set) || len(env) != 0 {
		t.Errorf("no file: %v", env)
	}
	t.Chdir(settingsDir(t, "A=b\n")) // an empty directory must not mean "the working directory"
	if !loadServiceSettings([]string{"backup"}, &bytes.Buffer{}, "", env.lookup, env.set) || len(env) != 0 {
		t.Errorf("no service directory: %v", env)
	}
}

func TestLoadServiceSettingsSkipsSomeCommands(t *testing.T) {
	dir := settingsDir(t, "not a setting\n") // would fail if it were read
	for _, args := range [][]string{{"service", "install"}, {"version"}, {"openapi"}, {}} {
		env := fakeEnv{}
		if !loadServiceSettings(args, &bytes.Buffer{}, dir, env.lookup, env.set) || len(env) != 0 {
			t.Errorf("%v must not read the file: %v", args, env)
		}
	}
}

func TestLoadServiceSettingsBadFile(t *testing.T) {
	dir := settingsDir(t, "LITURGIST_BASE_URL=ok\nnot a setting\n")
	env := fakeEnv{}
	var stderr bytes.Buffer
	if loadServiceSettings([]string{"backup"}, &stderr, dir, env.lookup, env.set) {
		t.Fatal("a bad file must stop the command")
	}
	if !strings.Contains(stderr.String(), "liturgist.env: line 2") || len(env) != 0 {
		t.Errorf("message %q, env %v", stderr.String(), env)
	}
}
