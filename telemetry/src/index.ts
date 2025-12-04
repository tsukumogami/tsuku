export interface Env {
  ANALYTICS: AnalyticsEngineDataset;
  CF_ACCOUNT_ID: string;
  CF_API_TOKEN: string;
  VERSION_TOKEN: string;
  COMMIT_SHA: string;
  DEPLOY_TIME: string;
}

const SCHEMA_VERSION = "1";

type ActionType = "install" | "update" | "remove" | "create" | "command";

interface TelemetryEvent {
  action: ActionType;
  recipe?: string;
  version_constraint?: string;
  version_resolved?: string;
  version_previous?: string;
  os?: string;
  arch?: string;
  tsuku_version?: string;
  is_dependency?: boolean;
  command?: string;
  flags?: string;
  template?: string;
}

function validateEvent(event: TelemetryEvent): string | null {
  const { action } = event;

  // Helper to check if a string field is set
  const hasString = (value: unknown): boolean =>
    typeof value === "string" && value !== "";

  // Helper to check if a field should be empty
  const mustBeEmpty = (field: string, value: unknown): string | null => {
    if (value !== undefined && value !== "" && value !== null) {
      return `${field} must be empty for ${action} action`;
    }
    return null;
  };

  // Common required fields for all actions
  if (!hasString(event.os)) {
    return "os is required";
  }
  if (!hasString(event.arch)) {
    return "arch is required";
  }
  if (!hasString(event.tsuku_version)) {
    return "tsuku_version is required";
  }

  switch (action) {
    case "install": {
      // Required: recipe, version_resolved
      if (!hasString(event.recipe)) return "recipe is required for install";
      if (!hasString(event.version_resolved))
        return "version_resolved is required for install";
      // Must be empty: command, flags, template
      let err = mustBeEmpty("command", event.command);
      if (err) return err;
      err = mustBeEmpty("flags", event.flags);
      if (err) return err;
      err = mustBeEmpty("template", event.template);
      if (err) return err;
      break;
    }
    case "update": {
      // Required: recipe, version_resolved, version_previous
      if (!hasString(event.recipe)) return "recipe is required for update";
      if (!hasString(event.version_resolved))
        return "version_resolved is required for update";
      if (!hasString(event.version_previous))
        return "version_previous is required for update";
      // Must be empty: is_dependency, command, flags, template
      if (event.is_dependency !== undefined)
        return "is_dependency must be empty for update action";
      let err = mustBeEmpty("command", event.command);
      if (err) return err;
      err = mustBeEmpty("flags", event.flags);
      if (err) return err;
      err = mustBeEmpty("template", event.template);
      if (err) return err;
      break;
    }
    case "remove": {
      // Required: recipe, version_previous
      if (!hasString(event.recipe)) return "recipe is required for remove";
      if (!hasString(event.version_previous))
        return "version_previous is required for remove";
      // Must be empty: version_constraint, version_resolved, is_dependency, command, flags, template
      let err = mustBeEmpty("version_constraint", event.version_constraint);
      if (err) return err;
      err = mustBeEmpty("version_resolved", event.version_resolved);
      if (err) return err;
      if (event.is_dependency !== undefined)
        return "is_dependency must be empty for remove action";
      err = mustBeEmpty("command", event.command);
      if (err) return err;
      err = mustBeEmpty("flags", event.flags);
      if (err) return err;
      err = mustBeEmpty("template", event.template);
      if (err) return err;
      break;
    }
    case "create": {
      // Required: template
      if (!hasString(event.template)) return "template is required for create";
      // Must be empty: recipe, version_*, is_dependency, command, flags
      let err = mustBeEmpty("recipe", event.recipe);
      if (err) return err;
      err = mustBeEmpty("version_constraint", event.version_constraint);
      if (err) return err;
      err = mustBeEmpty("version_resolved", event.version_resolved);
      if (err) return err;
      err = mustBeEmpty("version_previous", event.version_previous);
      if (err) return err;
      if (event.is_dependency !== undefined)
        return "is_dependency must be empty for create action";
      err = mustBeEmpty("command", event.command);
      if (err) return err;
      err = mustBeEmpty("flags", event.flags);
      if (err) return err;
      break;
    }
    case "command": {
      // Required: command
      if (!hasString(event.command))
        return "command field is required for command action";
      // Must be empty: recipe, version_*, is_dependency, template
      let err = mustBeEmpty("recipe", event.recipe);
      if (err) return err;
      err = mustBeEmpty("version_constraint", event.version_constraint);
      if (err) return err;
      err = mustBeEmpty("version_resolved", event.version_resolved);
      if (err) return err;
      err = mustBeEmpty("version_previous", event.version_previous);
      if (err) return err;
      if (event.is_dependency !== undefined)
        return "is_dependency must be empty for command action";
      err = mustBeEmpty("template", event.template);
      if (err) return err;
      break;
    }
  }

  return null;
}

const corsHeaders = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type",
};

interface AnalyticsRow {
  [key: string]: string | number;
}

interface AnalyticsResponse {
  data: AnalyticsRow[];
  meta: { name: string; type: string }[];
  rows: number;
}

async function queryAnalyticsEngine(
  env: Env,
  sql: string
): Promise<AnalyticsRow[]> {
  const response = await fetch(
    `https://api.cloudflare.com/client/v4/accounts/${env.CF_ACCOUNT_ID}/analytics_engine/sql`,
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${env.CF_API_TOKEN}`,
        "Content-Type": "text/plain",
      },
      body: sql,
    }
  );

  if (!response.ok) {
    const errorBody = await response.text();
    throw new Error(`Analytics Engine query failed: ${response.status} - ${errorBody}`);
  }

  const result = (await response.json()) as AnalyticsResponse;
  return result.data || [];
}

interface StatsResponse {
  generated_at: string;
  period: string;
  total_installs: number;
  recipes: { name: string; installs: number; updates: number }[];
  by_os: Record<string, number>;
  by_arch: Record<string, number>;
}

async function getStats(env: Env): Promise<StatsResponse> {
  // Query for total installs and recipe breakdown
  // Analytics Engine uses 1-indexed blobs: blob1=action, blob2=recipe, blob6=os, blob7=arch
  const recipeQuery = `
    SELECT blob2 as recipe,
           sum(if(blob1 = 'install', 1, 0)) as installs,
           sum(if(blob1 = 'update', 1, 0)) as updates
    FROM tsuku_telemetry
    WHERE blob2 != ''
    GROUP BY blob2
    ORDER BY installs DESC
    LIMIT 20
  `;

  // Query for OS breakdown
  const osQuery = `
    SELECT blob6 as os, count() as count
    FROM tsuku_telemetry
    WHERE blob1 = 'install'
    GROUP BY blob6
  `;

  // Query for architecture breakdown
  const archQuery = `
    SELECT blob7 as arch, count() as count
    FROM tsuku_telemetry
    WHERE blob1 = 'install'
    GROUP BY blob7
  `;

  const [recipeData, osData, archData] = await Promise.all([
    queryAnalyticsEngine(env, recipeQuery),
    queryAnalyticsEngine(env, osQuery),
    queryAnalyticsEngine(env, archQuery),
  ]);

  // Calculate total installs from recipe data
  const totalInstalls = recipeData.reduce(
    (sum, row) => sum + (Number(row.installs) || 0),
    0
  );

  // Transform recipe data
  const recipes = recipeData.map((row) => ({
    name: String(row.recipe),
    installs: Number(row.installs) || 0,
    updates: Number(row.updates) || 0,
  }));

  // Transform OS data
  const byOs: Record<string, number> = {};
  for (const row of osData) {
    const os = String(row.os);
    if (os && os !== "unknown") {
      byOs[os] = Number(row.count) || 0;
    }
  }

  // Transform arch data
  const byArch: Record<string, number> = {};
  for (const row of archData) {
    const arch = String(row.arch);
    if (arch && arch !== "unknown") {
      byArch[arch] = Number(row.count) || 0;
    }
  }

  return {
    generated_at: new Date().toISOString(),
    period: "all_time",
    total_installs: totalInstalls,
    recipes,
    by_os: byOs,
    by_arch: byArch,
  };
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);

    // Handle CORS preflight
    if (request.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders });
    }

    // POST /event - receive telemetry
    if (request.method === "POST" && url.pathname === "/event") {
      try {
        const event = (await request.json()) as Record<string, unknown>;

        // Validate required action field
        const validActions: ActionType[] = [
          "install",
          "update",
          "remove",
          "create",
          "command",
        ];
        if (
          typeof event.action !== "string" ||
          !validActions.includes(event.action as ActionType)
        ) {
          return new Response("Bad request: invalid action", {
            status: 400,
            headers: corsHeaders,
          });
        }

        // Validate event fields based on action type
        const telemetryEvent: TelemetryEvent = {
          action: event.action as ActionType,
          recipe: event.recipe as string | undefined,
          version_constraint: event.version_constraint as string | undefined,
          version_resolved: event.version_resolved as string | undefined,
          version_previous: event.version_previous as string | undefined,
          os: event.os as string | undefined,
          arch: event.arch as string | undefined,
          tsuku_version: event.tsuku_version as string | undefined,
          is_dependency: event.is_dependency as boolean | undefined,
          command: event.command as string | undefined,
          flags: event.flags as string | undefined,
          template: event.template as string | undefined,
        };
        const validationError = validateEvent(telemetryEvent);
        if (validationError) {
          return new Response(`Bad request: ${validationError}`, {
            status: 400,
            headers: corsHeaders,
          });
        }

        const action = event.action as ActionType;

        // Build 13-element blob array per schema
        const recipe = typeof event.recipe === "string" ? event.recipe : "";
        const index =
          action === "install" || action === "update" || action === "remove"
            ? recipe
            : action;

        env.ANALYTICS.writeDataPoint({
          blobs: [
            action, // blob0: action
            recipe, // blob1: recipe
            typeof event.version_constraint === "string"
              ? event.version_constraint
              : "", // blob2
            typeof event.version_resolved === "string"
              ? event.version_resolved
              : "", // blob3
            typeof event.version_previous === "string"
              ? event.version_previous
              : "", // blob4
            typeof event.os === "string" ? event.os : "", // blob5
            typeof event.arch === "string" ? event.arch : "", // blob6
            typeof event.tsuku_version === "string" ? event.tsuku_version : "", // blob7
            typeof event.is_dependency === "boolean"
              ? String(event.is_dependency)
              : "", // blob8
            typeof event.command === "string" ? event.command : "", // blob9
            typeof event.flags === "string" ? event.flags : "", // blob10
            typeof event.template === "string" ? event.template : "", // blob11
            SCHEMA_VERSION, // blob12
          ],
          indexes: [index],
        });

        return new Response("ok", { status: 200, headers: corsHeaders });
      } catch {
        return new Response("Bad request: invalid JSON", {
          status: 400,
          headers: corsHeaders,
        });
      }
    }

    // GET /stats - return aggregated statistics
    if (request.method === "GET" && url.pathname === "/stats") {
      try {
        const stats = await getStats(env);
        return new Response(JSON.stringify(stats), {
          status: 200,
          headers: { ...corsHeaders, "Content-Type": "application/json" },
        });
      } catch (error) {
        return new Response(JSON.stringify({ error: String(error) }), {
          status: 500,
          headers: { ...corsHeaders, "Content-Type": "application/json" },
        });
      }
    }

    // GET /health - health check
    if (url.pathname === "/health") {
      return new Response("ok", { status: 200, headers: corsHeaders });
    }

    // GET /version - deployment info (protected by token)
    if (request.method === "GET" && url.pathname === "/version") {
      const authHeader = request.headers.get("Authorization");
      const expectedToken = env.VERSION_TOKEN;

      if (!expectedToken) {
        return new Response("Version endpoint not configured", {
          status: 503,
          headers: corsHeaders,
        });
      }

      if (!authHeader || authHeader !== `Bearer ${expectedToken}`) {
        return new Response("Unauthorized", {
          status: 401,
          headers: corsHeaders,
        });
      }

      return new Response(
        JSON.stringify({
          commit_sha: env.COMMIT_SHA || "unknown",
          deploy_time: env.DEPLOY_TIME || "unknown",
          schema_version: SCHEMA_VERSION,
        }),
        {
          status: 200,
          headers: { ...corsHeaders, "Content-Type": "application/json" },
        }
      );
    }

    return new Response("Not found", { status: 404, headers: corsHeaders });
  },
};
