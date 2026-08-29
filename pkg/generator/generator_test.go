package generator

import (
	"strings"
	"testing"
)

func TestGenerateRunCmd(t *testing.T) {
	meta := MapMeta{
		"name":           "web-srv",
		"image":          "nginx:alpine",
		"restartPolicy":  "always",
		"ports":          "8080 -> 80/tcp",
		"[MOUNTS]":       "/usr/share/nginx/html:::/var/www/html:::volume:::ro",
		"[ENV-VAR]":      "ENV=production;DEBUG=false",
		"memLimit":       "512 MB",
		"cpuLimit":       "2.0 (200%)",
		"pidsLimit":      "100",
		"privileged":     "true",
		"readonlyRootfs": "true",
		"cmd":            "nginx -g 'daemon off;'",
	}

	cmd := GenerateRunCmd(meta)
	if !strings.Contains(cmd, "docker run -d") {
		t.Errorf("expected 'docker run -d', got: %s", cmd)
	}
	if !strings.Contains(cmd, "--name web-srv") {
		t.Errorf("expected --name web-srv, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--restart always") {
		t.Errorf("expected --restart always, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-p 8080:80/tcp") {
		t.Errorf("expected -p 8080:80/tcp, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-v /var/www/html:/usr/share/nginx/html:ro") {
		t.Errorf("expected -v /var/www/html:/usr/share/nginx/html:ro, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-e \"ENV=production\"") {
		t.Errorf("expected -e \"ENV=production\", got: %s", cmd)
	}
	if !strings.Contains(cmd, "--memory 512MB") {
		t.Errorf("expected --memory 512MB, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--cpus 2.0") {
		t.Errorf("expected --cpus 2.0, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--pids-limit 100") {
		t.Errorf("expected --pids-limit 100, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--privileged") {
		t.Errorf("expected --privileged, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--read-only") {
		t.Errorf("expected --read-only, got: %s", cmd)
	}
	if !strings.Contains(cmd, "nginx:alpine") {
		t.Errorf("expected image nginx:alpine, got: %s", cmd)
	}
}

func TestGenerateCompose(t *testing.T) {
	meta := MapMeta{
		"name":           "web-srv",
		"image":          "nginx:alpine",
		"restartPolicy":  "always",
		"ports":          "8080 -> 80/tcp",
		"[MOUNTS]":       "/usr/share/nginx/html:::/var/www/html:::volume:::ro",
		"[ENV-VAR]":      "ENV=production;DEBUG=false",
		"privileged":     "true",
		"readonlyRootfs": "true",
		"cmd":            "nginx -g 'daemon off;'",
	}

	compose := GenerateCompose(meta)
	if !strings.Contains(compose, "version: '3.8'") {
		t.Errorf("expected version 3.8, got: %s", compose)
	}
	if !strings.Contains(compose, "web-srv:") {
		t.Errorf("expected service web-srv, got: %s", compose)
	}
	if !strings.Contains(compose, "image: nginx:alpine") {
		t.Errorf("expected image nginx:alpine, got: %s", compose)
	}
	if !strings.Contains(compose, "restart: always") {
		t.Errorf("expected restart always, got: %s", compose)
	}
	if !strings.Contains(compose, "- \"8080:80/tcp\"") {
		t.Errorf("expected port mapping, got: %s", compose)
	}
	if !strings.Contains(compose, "- /var/www/html:/usr/share/nginx/html:ro") {
		t.Errorf("expected volume mount, got: %s", compose)
	}
	if !strings.Contains(compose, "privileged: true") {
		t.Errorf("expected privileged: true, got: %s", compose)
	}
	if !strings.Contains(compose, "read_only: true") {
		t.Errorf("expected read_only: true, got: %s", compose)
	}
}
