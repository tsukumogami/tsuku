-- Batch validation metrics schema
-- Stores results from CI recipe validation runs

CREATE TABLE batch_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id TEXT NOT NULL,
    ecosystem TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    total_recipes INTEGER NOT NULL DEFAULT 0,
    passed INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    skipped INTEGER NOT NULL DEFAULT 0,
    success_rate REAL NOT NULL DEFAULT 0.0,
    macos_minutes REAL NOT NULL DEFAULT 0.0,
    linux_minutes REAL NOT NULL DEFAULT 0.0
);

CREATE TABLE recipe_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_run_id INTEGER NOT NULL,
    recipe_name TEXT NOT NULL,
    ecosystem TEXT NOT NULL,
    result TEXT NOT NULL,
    error_category TEXT,
    error_message TEXT,
    duration_seconds REAL NOT NULL DEFAULT 0.0,
    FOREIGN KEY (batch_run_id) REFERENCES batch_runs(id)
);

CREATE INDEX idx_batch_runs_batch_id ON batch_runs(batch_id);
CREATE INDEX idx_batch_runs_ecosystem ON batch_runs(ecosystem);
CREATE INDEX idx_recipe_results_batch_run_id ON recipe_results(batch_run_id);
CREATE INDEX idx_recipe_results_recipe_name ON recipe_results(recipe_name);
CREATE INDEX idx_recipe_results_result ON recipe_results(result);
