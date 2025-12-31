# gomento

## Problem

If your agent solves a complex bug today, and you ask it to solve the same bug next week, it starts from scratch. It doesn't remember the solution; it just has a massive log file that it can't read efficiently.

Agents make the same mistakes repeatedly because their successes are buried in raw logs. They don't learn; they just execute.

## Solution

GoMento is a high-performance, single-binary sidecar written in **Go**. It accepts raw chat logs, uses a background worker to **distill** those logs into SOPs (Standard Operating Procedures), and makes them searchable for your agent next time.

### Usage

```bash
docker compose up
make migrate-up
go run main.go server
```

### Architecture

```mermaid
graph TD
    subgraph "Your Runtime"
        User[User] <-->|Chat + Files| Agent[Your Agent]
    end

    subgraph "GoMento"
        API[API Gateway]
        Worker[Background Worker]
    end

    subgraph "Infrastructure"
        Postgres[Postgres + pgvector]
        MinIO[MinIO/S3]
        LLM[LLM Provider]
    end

    %% Flow 1: Storing Context
    Agent -- "1. Push Chat Logs, Files, and Assets" --> API
    API -- "2. Upload Assets" --> MinIO
    API -- "3. Save Messages, Files, and Asset Metadata" --> Postgres
    
    %% Flow 2: Interpretation
    Agent -- "4. Extract/Distill" --> API
    API -- "5. Produce Job" --> Worker
    Worker -- "6. Trigger Interpretation" --> LLM
    Worker -- "7. Save Tasks/Skills" --> Postgres

    %% Flow 3: Retrieval (Current Session History)
    Agent -- "8. Get Tasks & Messages (w/ Assets)" --> API
    API -- "9. Fetch History" --> Postgres
    API -- "10. Presign URLs" --> MinIO
    API -- "11. Return History" --> Agent

    %% Flow 4: Retrieval (Skills)
    Agent -- "12. Ask: 'How do I fix Redis?'" --> API
    API -- "13. Vector Search" --> Postgres
    API -- "14. Return SOP" --> Agent
```

### ER Diagram

```mermaid
erDiagram
    PROJECTS {
        UUID id PK
        TEXT name
        TIMESTAMPTZ created_at
    }

    SPACES {
        UUID id PK
        UUID project_id FK
        TEXT name
        TIMESTAMPTZ created_at
    }

    SKILLS {
        UUID id PK
        UUID space_id FK
        TEXT trigger "The problem summary"
        TEXT sop "The solution steps"
        VECTOR embedding "pgvector(1536)"
        TIMESTAMPTZ created_at
    }

    SESSIONS {
        UUID id PK
        UUID project_id FK
        UUID space_id FK "Nullable"
        TIMESTAMPTZ created_at
    }

    TASKS {
        UUID id PK
        UUID session_id FK
        INT task_order "Order of task execution within a session"
        BOOLEAN is_thought "Whether or not this task is associated with thought messages or action messages"
        JSON data "Task-specific payload"
        VARCHAR status "pending|running|success|failed"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    MESSAGES {
        UUID id PK
        UUID session_id FK
        UUID tasks_id FK "Nullable"
        UUID parent_id FK "Nullable, self-reference"
        VARCHAR role "'user' or 'assistant'"
        JSONB parts "Stores [{'type':'text'}, {'type':'image'}]"
        TIMESTAMPTZ created_at
    }

    MESSAGE_ASSETS {
        UUID message_id PK, FK
        UUID asset_id PK, FK
    }

    ASSETS {
        UUID id PK
        TEXT container "e.g., Bucket"
        TEXT path "Path to file"
        TEXT etag
        TEXT sha256 "Deduplication hash"
        TEXT mime "e.g. image/png"
        BIGINT size_bytes
        TIMESTAMPTZ created_at
    }

    ARTIFACTS {
        UUID id PK
        UUID project_id FK
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    FILES {
        UUID id PK
        UUID artifact_id FK
        UUID asset_id FK
        TEXT path
        TEXT filename
        JSONB meta
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    PROJECTS ||--|{ SPACES : ""
    PROJECTS ||--|{ SESSIONS : ""
    PROJECTS ||--|{ ARTIFACTS : ""
    
    SPACES ||--o{ SKILLS : ""
    SPACES |o--o{ SESSIONS : ""
    
    SESSIONS ||--o{ TASKS : ""
    TASKS |o--o{ MESSAGES : ""
    SESSIONS ||--o{ MESSAGES : ""

    MESSAGES ||--o{ MESSAGE_ASSETS : ""
    ASSETS ||--o{ MESSAGE_ASSETS : ""

    ARTIFACTS ||--o{ FILES : ""
    ASSETS ||--o{ FILES : ""
```