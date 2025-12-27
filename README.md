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
        Queue[Queue/Channel]
        Worker[Background Worker]
    end

    subgraph "Infrastructure"
        Postgres[Postgres + pgvector]
        MinIO[MinIO/S3]
        LLM[LLM Provider]
    end

    %% Flow 1: Storing Context
    Agent -- "1. Push Chat Logs & Assets" --> API
    API -- "2. Upload Assets" --> MinIO
    API -- "3. Save" --> Postgres
    API -- "4. Persist Job" --> Postgres
    API -- "5. Produce Job" --> Queue
    
    %% Flow 2: Distillation
    Queue -- "6. Consume" --> Worker
    Worker -- "7. Update Job (Running)" --> Postgres
    Worker -- "8. Fetch Context" --> Postgres
    Worker -- "9. Distill (Extract SOP)" --> LLM
    Worker -- "10. Save Skill/Vector" --> Postgres
    Worker -- "11. Update Job" --> Postgres

    %% Flow 3: Retrieval (Skills)
    Agent -- "12. Ask: 'How do I fix Redis?'" --> API
    API -- "13. Vector Search" --> Postgres
    API -- "14. Return SOP" --> Agent

    %% Flow 4: Retrieval (Messages + Assets)
    Agent -- "15. Get History (w/ Assets)" --> API
    API -- "16. Fetch Messages" --> Postgres
    API -- "17. Presign URLs" --> MinIO
    API -- "18. Return History" --> Agent
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
        BOOLEAN is_analyzing "Whether or not this task is currently being analyzed by the worker/agent"
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

    MESSAGE_ASSETS {
        UUID message_id PK, FK
        UUID asset_id PK, FK
    }

    PROJECTS ||--|{ SPACES : ""
    PROJECTS ||--|{ SESSIONS : ""
    
    SPACES ||--|{ SKILLS : ""
    SPACES |o--o{ SESSIONS : ""
    
    SESSIONS ||--|{ MESSAGES : ""
    SESSIONS ||--|{ TASKS : ""

    TASKS |o--o{ MESSAGES : ""

    MESSAGES ||--|{ MESSAGE_ASSETS : ""
    ASSETS ||--|{ MESSAGE_ASSETS : ""
```