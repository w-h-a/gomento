# gomento

## Problem

If your agent solves a complex bug today, and you ask it to solve the same bug next week, it starts from scratch. It doesn't remember the solution; it just has a massive log file that it can't read efficiently.

Agents make the same mistakes repeatedly because their successes are buried in raw logs. They don't learn; they just execute.

## Solution

GoMento is a high-performance, single-binary sidecar written in **Go**. It accepts raw chat logs and files, uses a background worker to (a) **extract** a summary of the current session and (b) **distill** the current session into skills that are not session-bound, and makes all this memory easily searchable for your agent.

### Usage

```bash
docker compose -f docker-compose.demo.yml up
make migrate-up
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

    %% Flow 3: Retrieval (Within a Session)
    Agent -- "8. Get Tasks & Messages Within Session" --> API
    API -- "9. Fetch Session History" --> Postgres
    API -- "10. Presign URLs" --> MinIO
    API -- "11. Return Session History" --> Agent

    %% Flow 4: Retrieval (Across Sessions)
    Agent -- "12. Get Skills & Messages Across Sessions" --> API
    API -- "13. Vector Search" --> Postgres
    API -- "14. Return Relevant Skills/Messages" --> Agent
```

### ER Diagram

```mermaid
erDiagram
    SPACES {
        UUID id PK
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
        UUID space_id FK "Nullable"
        TIMESTAMPTZ created_at
    }

    TASKS {
        UUID id PK
        UUID session_id FK
        INT task_order "Order of task execution within a session"
        BOOLEAN is_thought
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
        VECTOR embedding "pgvector(1536)"
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

    FILES {
        UUID id PK
        UUID space_id FK "Nullable"
        UUID asset_id FK
        TEXT path
        TEXT filename
        JSONB meta
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }
    
    SPACES ||--o{ SKILLS : ""
    SPACES |o--o{ SESSIONS : ""
    SPACES |o--o{ FILES: ""
    
    SESSIONS ||--o{ TASKS : ""
    TASKS |o--o{ MESSAGES : ""
    SESSIONS ||--o{ MESSAGES : ""

    MESSAGES ||--o{ MESSAGE_ASSETS : ""
    ASSETS ||--o{ MESSAGE_ASSETS : ""

    ASSETS ||--o{ FILES : ""
```