import { describe, it, expect, beforeAll, beforeEach, afterEach } from "vitest";
import { env, SELF, fetchMock } from "cloudflare:test";

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

  describe("POST /event (LLM events)", () => {
    // Common fields for LLM events
    const baseFields = {
      os: "linux",
      arch: "amd64",
      tsuku_version: "0.3.0",
    };

    // Valid LLM events
    it("returns ok for valid llm_generation_started event", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          provider: "claude",
          tool_name: "serve",
          repo: "owner/repo",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for valid llm_generation_completed event", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_completed",
          provider: "gemini",
          tool_name: "serve",
          success: true,
          duration_ms: 1500,
          attempts: 2,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for valid llm_repair_attempt event", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_repair_attempt",
          provider: "claude",
          attempt_number: 2,
          error_category: "install_failure",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for valid llm_validation_result event", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_validation_result",
          passed: true,
          attempt_number: 1,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    it("returns ok for valid llm_circuit_breaker_trip event", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_circuit_breaker_trip",
          provider: "claude",
          failures: 3,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    });

    // Validation: missing common fields
    it("returns 400 for LLM event missing os", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          provider: "claude",
          tool_name: "serve",
          repo: "owner/repo",
          arch: "amd64",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("os is required");
    });

    it("returns 400 for LLM event missing arch", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          provider: "claude",
          tool_name: "serve",
          repo: "owner/repo",
          os: "linux",
          tsuku_version: "0.3.0",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("arch is required");
    });

    it("returns 400 for LLM event missing tsuku_version", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          provider: "claude",
          tool_name: "serve",
          repo: "owner/repo",
          os: "linux",
          arch: "amd64",
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("tsuku_version is required");
    });

    // Validation: llm_generation_started required fields
    it("returns 400 for llm_generation_started missing provider", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          tool_name: "serve",
          repo: "owner/repo",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("provider is required");
    });

    it("returns 400 for llm_generation_started missing tool_name", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          provider: "claude",
          repo: "owner/repo",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("tool_name is required");
    });

    it("returns 400 for llm_generation_started missing repo", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_started",
          provider: "claude",
          tool_name: "serve",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("repo is required");
    });

    // Validation: llm_generation_completed required fields
    it("returns 400 for llm_generation_completed missing provider", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_completed",
          tool_name: "serve",
          success: true,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("provider is required");
    });

    it("returns 400 for llm_generation_completed missing tool_name", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_generation_completed",
          provider: "claude",
          success: true,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("tool_name is required");
    });

    // Validation: llm_repair_attempt required fields
    it("returns 400 for llm_repair_attempt missing provider", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_repair_attempt",
          attempt_number: 2,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("provider is required");
    });

    it("returns 400 for llm_repair_attempt missing attempt_number", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_repair_attempt",
          provider: "claude",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("attempt_number is required");
    });

    // Validation: llm_validation_result required fields
    it("returns 400 for llm_validation_result missing attempt_number", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_validation_result",
          passed: true,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("attempt_number is required");
    });

    // Validation: llm_circuit_breaker_trip required fields
    it("returns 400 for llm_circuit_breaker_trip missing provider", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_circuit_breaker_trip",
          failures: 3,
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("provider is required");
    });

    it("returns 400 for llm_circuit_breaker_trip missing failures", async () => {
      const response = await SELF.fetch("http://localhost/event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action: "llm_circuit_breaker_trip",
          provider: "claude",
          ...baseFields,
        }),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("failures is required");
    });
  });

  describe("POST /batch-metrics", () => {
    beforeAll(async () => {
      const db = env.BATCH_METRICS;
      await db.exec("CREATE TABLE IF NOT EXISTS batch_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, batch_id TEXT NOT NULL, ecosystem TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT, total_recipes INTEGER NOT NULL DEFAULT 0, passed INTEGER NOT NULL DEFAULT 0, failed INTEGER NOT NULL DEFAULT 0, skipped INTEGER NOT NULL DEFAULT 0, success_rate REAL NOT NULL DEFAULT 0.0, macos_minutes REAL NOT NULL DEFAULT 0.0, linux_minutes REAL NOT NULL DEFAULT 0.0)");
      await db.exec("CREATE TABLE IF NOT EXISTS recipe_results (id INTEGER PRIMARY KEY AUTOINCREMENT, batch_run_id INTEGER NOT NULL, recipe_name TEXT NOT NULL, ecosystem TEXT NOT NULL, result TEXT NOT NULL, error_category TEXT, error_message TEXT, duration_seconds REAL NOT NULL DEFAULT 0.0, FOREIGN KEY (batch_run_id) REFERENCES batch_runs(id))");
    });

    const validPayload = {
      batch_id: "2026-01-28-001",
      ecosystem: "homebrew",
      started_at: "2026-01-28T10:00:00Z",
      completed_at: "2026-01-28T10:30:00Z",
      total_recipes: 3,
      passed: 2,
      failed: 1,
      skipped: 0,
      success_rate: 0.667,
      macos_minutes: 15.5,
      linux_minutes: 8.2,
      results: [
        {
          recipe_name: "wget",
          ecosystem: "homebrew",
          result: "passed",
          duration_seconds: 45.2,
        },
        {
          recipe_name: "curl",
          ecosystem: "homebrew",
          result: "passed",
          duration_seconds: 30.1,
        },
        {
          recipe_name: "jq",
          ecosystem: "homebrew",
          result: "failed",
          error_category: "validation",
          error_message: "checksum mismatch",
          duration_seconds: 12.0,
        },
      ],
    };

    const authHeaders = {
      "Content-Type": "application/json",
      Authorization: "Bearer test-batch-metrics-token",
    };

    it("returns 401 without authorization header", async () => {
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(validPayload),
      });
      expect(response.status).toBe(401);
      expect(await response.text()).toBe("Unauthorized");
    });

    it("returns 401 with invalid token", async () => {
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer wrong-token",
        },
        body: JSON.stringify(validPayload),
      });
      expect(response.status).toBe(401);
      expect(await response.text()).toBe("Unauthorized");
    });

    it("returns 400 for invalid JSON", async () => {
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: "not json",
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("invalid JSON");
    });

    it("returns 400 for missing batch_id", async () => {
      const { batch_id, ...payload } = validPayload;
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(payload),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("batch_id is required");
    });

    it("returns 400 for missing ecosystem", async () => {
      const { ecosystem, ...payload } = validPayload;
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(payload),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("ecosystem is required");
    });

    it("returns 400 for missing started_at", async () => {
      const { started_at, ...payload } = validPayload;
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(payload),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("started_at is required");
    });

    it("returns 400 for missing results array", async () => {
      const { results, ...payload } = validPayload;
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(payload),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("results array is required");
    });

    it("returns 400 for recipe result missing recipe_name", async () => {
      const payload = {
        ...validPayload,
        results: [{ ecosystem: "homebrew", result: "passed" }],
      };
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(payload),
      });
      expect(response.status).toBe(400);
      expect(await response.text()).toContain("recipe_name is required");
    });

    it("returns 201 for valid payload with results", async () => {
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(validPayload),
      });
      expect(response.status).toBe(201);
      const data = (await response.json()) as { batch_run_id: number };
      expect(data.batch_run_id).toBeDefined();
      expect(typeof data.batch_run_id).toBe("number");
    });

    it("returns 201 for valid payload with empty results", async () => {
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify({ ...validPayload, results: [] }),
      });
      expect(response.status).toBe(201);
      const data = (await response.json()) as { batch_run_id: number };
      expect(data.batch_run_id).toBeDefined();
    });

    it("includes CORS headers", async () => {
      const response = await SELF.fetch("http://localhost/batch-metrics", {
        method: "POST",
        headers: authHeaders,
        body: JSON.stringify(validPayload),
      });
      expect(response.headers.get("Access-Control-Allow-Origin")).toBe("*");
    });
  });

  describe("unknown routes", () => {
    it("returns 404 for unknown paths", async () => {
      const response = await SELF.fetch("http://localhost/unknown");
      expect(response.status).toBe(404);
    });
  });
});
