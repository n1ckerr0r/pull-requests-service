CREATE TABLE IF NOT EXISTS teams (
    name TEXT PRIMARY KEY,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    user_id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    team_name TEXT NOT NULL REFERENCES teams(name) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

CREATE TABLE IF NOT EXISTS prs (
    pull_request_id TEXT PRIMARY KEY,
    pull_request_name TEXT NOT NULL,
    author_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    merged_at TIMESTAMP WITH TIME ZONE NULL
);

CREATE TABLE IF NOT EXISTS pr_assignments (
    pull_request_id TEXT NOT NULL REFERENCES prs(pull_request_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    slot SMALLINT NOT NULL CHECK (slot IN (1,2)),
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    PRIMARY KEY (pull_request_id, slot),
    UNIQUE (pull_request_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_users_team_active ON users (team_name, is_active);
CREATE INDEX IF NOT EXISTS idx_pr_assignments_pr ON pr_assignments (pull_request_id);
