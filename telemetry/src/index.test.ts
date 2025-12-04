import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { SELF, fetchMock } from "cloudflare:test";

describe("tsuku-telemetry worker", () => {
  beforeEach(() => {
    fetchMock.activate();
    fetchMock.disableNetConnect();
  });

  afterEach(() => {
    fetchMock.deactivate();
  });

  describe("CORS", () => {
    it("handles OPTIONS preflight requests", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "OPTIONS",
      });
      expect(response.status).toBe(200);
      expect(response.headers.get("Access-Control-Allow-Origin")).toBe("*");
      expect(response.headers.get("Access-Control-Allow-Methods")).toBe(
        "GET, POST, OPTIONS"
      );
    });

    it("includes CORS headers on all responses", async () => {
      const response = await SELF.fetch("http://localhost/health");
      expect(response.headers.get("Access-Control-Allow-Origin")).toBe("*");
    });
  });

  describe("GET /health", () => {
    it("returns ok", async () => {
      const response = await SELF.fetch("http://localhost/health");
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });
  });

  describe("GET /version", () => {
    it("returns 401 without authorization header", async () => {
      const response = await SELF.fetch("http://localhost/version");
      expect(response.status).toBe(401);
      expect(await response.text()).toBe("Unauthorized");
    });

    it("returns 401 with invalid token", async () => {
      const response = await SELF.fetch("http://localhost/version", {
        headers: { Authorization: "Bearer wrong-token" },
      });
      expect(response.status).toBe(401);
      expect(await response.text()).toBe("Unauthorized");
    });

    it("returns version info with valid token", async () => {
      const response = await SELF.fetch("http://localhost/version", {
        headers: { Authorization: "Bearer test-version-token" },
      });
      expect(response.status).toBe(200);
      expect(response.headers.get("Content-Type")).toBe("application/json");

      const data = (await response.json()) as {
        commit_sha: string;
        deploy_time: string;
        schema_version: string;
      };
      expect(data.commit_sha).toBeDefined();
      expect(data.deploy_time).toBeDefined();
      expect(data.schema_version).toBe("1");
    });

    it("includes CORS headers", async () => {
      const response = await SELF.fetch("http://localhost/version", {
        headers: { Authorization: "Bearer test-version-token" },
      });
      expect(response.headers.get("Access-Control-Allow-Origin")).toBe("*");
    });
  });

  describe("GET /stats", () => {
    it("returns aggregated statistics", async () => {
      // Mock the Analytics Engine API responses
      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [
              { recipe: "nodejs", installs: 100, updates: 10 },
              { recipe: "terraform", installs: 50, updates: 5 },
            ],
            meta: [],
            rows: 2,
          })
        )
        .times(1);

      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [
              { os: "linux", count: 100 },
              { os: "darwin", count: 50 },
            ],
            meta: [],
            rows: 2,
          })
        )
        .times(1);

      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [
              { arch: "amd64", count: 120 },
              { arch: "arm64", count: 30 },
            ],
            meta: [],
            rows: 2,
          })
        )
        .times(1);

      const response = await SELF.fetch("http://localhost/stats");
      expect(response.status).toBe(200);
      expect(response.headers.get("Content-Type")).toBe("application/json");

      const stats = (await response.json()) as {
        generated_at: string;
        period: string;
        total_installs: number;
        recipes: { name: string; installs: number; updates: number }[];
        by_os: Record<string, number>;
        by_arch: Record<string, number>;
      };

      expect(stats.generated_at).toBeDefined();
      expect(stats.period).toBe("all_time");
      expect(stats.total_installs).toBe(150);
      expect(stats.recipes).toHaveLength(2);
      expect(stats.recipes[0]).toEqual({
        name: "nodejs",
        installs: 100,
        updates: 10,
      });
      expect(stats.by_os).toEqual({ linux: 100, darwin: 50 });
      expect(stats.by_arch).toEqual({ amd64: 120, arm64: 30 });
    });

    it("filters out unknown OS and arch values", async () => {
      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [{ recipe: "nodejs", installs: 10, updates: 1 }],
            meta: [],
            rows: 1,
          })
        )
        .times(1);

      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [
              { os: "linux", count: 5 },
              { os: "unknown", count: 3 },
              { os: "", count: 2 },
            ],
            meta: [],
            rows: 3,
          })
        )
        .times(1);

      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [
              { arch: "amd64", count: 5 },
              { arch: "unknown", count: 3 },
              { arch: "", count: 2 },
            ],
            meta: [],
            rows: 3,
          })
        )
        .times(1);

      const response = await SELF.fetch("http://localhost/stats");
      expect(response.status).toBe(200);

      const stats = (await response.json()) as {
        by_os: Record<string, number>;
        by_arch: Record<string, number>;
      };

      // Should only include linux, not unknown or empty
      expect(stats.by_os).toEqual({ linux: 5 });
      expect(stats.by_arch).toEqual({ amd64: 5 });
    });

    it("handles empty data from API", async () => {
      // Test when API returns no data property (undefined)
      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            meta: [],
            rows: 0,
          })
        )
        .times(3);

      const response = await SELF.fetch("http://localhost/stats");
      expect(response.status).toBe(200);

      const stats = (await response.json()) as {
        total_installs: number;
        recipes: unknown[];
        by_os: Record<string, number>;
        by_arch: Record<string, number>;
      };

      expect(stats.total_installs).toBe(0);
      expect(stats.recipes).toEqual([]);
      expect(stats.by_os).toEqual({});
      expect(stats.by_arch).toEqual({});
    });

    it("handles non-numeric values gracefully", async () => {
      // Test when API returns non-numeric values that need || 0 fallback
      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [
              { recipe: "nodejs", installs: "not a number", updates: null },
            ],
            meta: [],
            rows: 1,
          })
        )
        .times(1);

      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [{ os: "linux", count: undefined }],
            meta: [],
            rows: 1,
          })
        )
        .times(1);

      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(
          200,
          JSON.stringify({
            data: [{ arch: "amd64", count: "invalid" }],
            meta: [],
            rows: 1,
          })
        )
        .times(1);

      const response = await SELF.fetch("http://localhost/stats");
      expect(response.status).toBe(200);

      const stats = (await response.json()) as {
        total_installs: number;
        recipes: { name: string; installs: number; updates: number }[];
        by_os: Record<string, number>;
        by_arch: Record<string, number>;
      };

      // Should default to 0 for non-numeric values
      expect(stats.total_installs).toBe(0);
      expect(stats.recipes[0].installs).toBe(0);
      expect(stats.recipes[0].updates).toBe(0);
      expect(stats.by_os.linux).toBe(0);
      expect(stats.by_arch.amd64).toBe(0);
    });

    it("returns 500 on API error", async () => {
      fetchMock
        .get("https://api.cloudflare.com")
        .intercept({
          path: /\/client\/v4\/accounts\/.*\/analytics_engine\/sql/,
          method: "POST",
        })
        .reply(401, "Unauthorized")
        .times(3);

      const response = await SELF.fetch("http://localhost/stats");
      expect(response.status).toBe(500);

      const error = (await response.json()) as { error: string };
      expect(error.error).toContain("Analytics Engine query failed");
    });
  });

  describe("POST /event", () => {
    it("returns ok for valid install event", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "test",
          action: "install",
          version_resolved: "1.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for install with optional version_constraint", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_constraint: "@LTS",
          version_resolved: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns 400 for missing recipe", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: "install" }),
      });
      expect(response.status).toBe(400);
    });

    it("returns 400 for missing action", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ recipe: "test" }),
      });
      expect(response.status).toBe(400);
    });

    it("returns 400 for invalid JSON", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "not json",
      });
      expect(response.status).toBe(400);
    });

    it("returns 400 for non-string recipe", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ recipe: 123, action: "install" }),
      });
      expect(response.status).toBe(400);
    });

    it("returns 400 for invalid action type", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ recipe: "test", action: "invalid" }),
      });
      expect(response.status).toBe(400);
    });

    it("returns ok for update action", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "update",
          version_resolved: "22.1.0",
          version_previous: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for remove action", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "remove",
          version_previous: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for create action with template", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "create",
          template: "github_release",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns 400 for create action without template", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "create",
          os: "linux",
        }),
      });
      expect(response.status).toBe(400);
    });

    it("returns ok for command action", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "command",
          command: "list",
          flags: "--json",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns 400 for command action without command field", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "command",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("command field is required");
    });

    it("returns ok for install with enhanced fields", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_constraint: "@LTS",
          version_resolved: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
          is_dependency: false,
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    // Validation: missing required common fields
    it("returns 400 for missing os", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_resolved: "22.0.0",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("os is required");
    });

    it("returns 400 for missing arch", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_resolved: "22.0.0",
          os: "linux",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("arch is required");
    });

    it("returns 400 for missing tsuku_version", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_resolved: "22.0.0",
          os: "linux",
          arch: "amd64",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("tsuku_version is required");
    });

    // Validation: install action - missing required fields
    it("returns 400 for install without version_resolved", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("version_resolved is required");
    });

    // Validation: install action - must-be-empty violations
    it("returns 400 for install with command field", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_resolved: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
          command: "list",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("command must be empty");
    });

    it("returns 400 for install with template field", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "install",
          version_resolved: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
          template: "github_release",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("template must be empty");
    });

    // Validation: update action - missing required fields
    it("returns 400 for update without version_previous", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "update",
          version_resolved: "22.1.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("version_previous is required");
    });

    // Validation: update action - must-be-empty violations
    it("returns 400 for update with is_dependency", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "update",
          version_resolved: "22.1.0",
          version_previous: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
          is_dependency: true,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("is_dependency must be empty");
    });

    // Validation: remove action - must-be-empty violations
    it("returns 400 for remove with version_resolved", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "remove",
          version_resolved: "22.0.0",
          version_previous: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("version_resolved must be empty");
    });

    // Validation: create action - must-be-empty violations
    it("returns 400 for create with recipe", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "create",
          template: "github_release",
          recipe: "my-tool",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("recipe must be empty");
    });

    // Validation: command action - must-be-empty violations
    it("returns 400 for command with recipe", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "command",
          command: "list",
          recipe: "nodejs",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("recipe must be empty");
    });

    it("returns 400 for command with template", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "command",
          command: "list",
          template: "github_release",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("template must be empty");
    });

    it("returns 400 for command with is_dependency", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "command",
          command: "list",
          is_dependency: true,
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("is_dependency must be empty");
    });

    // Additional validation tests for full coverage
    it("returns 400 for update without version_resolved", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "update",
          version_previous: "22.0.0",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("version_resolved is required");
    });

    it("returns 400 for remove without version_previous", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "remove",
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("version_previous is required");
    });

    it("returns 400 for remove with is_dependency", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recipe: "nodejs",
          action: "remove",
          version_previous: "22.0.0",
          is_dependency: true,
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("is_dependency must be empty");
    });

    it("returns 400 for create with is_dependency", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "create",
          template: "github_release",
          is_dependency: false,
          os: "linux",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("is_dependency must be empty");
    });
  });

  describe("unknown routes", () => {
    it("returns 404 for unknown paths", async () => {
      const response = await SELF.fetch("http://localhost/unknown");
      expect(response.status).toBe(404);
    });
  });
});
