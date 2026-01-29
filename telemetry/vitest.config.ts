import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    coverage: {
      provider: "istanbul",
      reporter: ["text", "lcov"],
    },
    poolOptions: {
      workers: {
        wrangler: { configPath: "./wrangler.toml" },
        miniflare: {
          bindings: {
            VERSION_TOKEN: "test-version-token",
            BATCH_METRICS_TOKEN: "test-batch-metrics-token",
            COMMIT_SHA: "test-commit-sha",
            DEPLOY_TIME: "2024-01-01T00:00:00Z",
          },
        },
      },
    },
  },
});
