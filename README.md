# gomento

<div align="center">
  <img src="./.github/assets/gomento.png" alt="GoMento Mascot" width="200" />
</div>

## Problem

If your agent solves a complex bug today, and you ask it to solve the same bug next week, it starts from scratch. It doesn't remember the solution; it just has a massive log file that it can't read efficiently.

Agents make the same mistakes repeatedly because their successes are buried in raw logs. They don't learn; they just execute.

## Solution

GoMento is a high-performance, single-binary sidecar for AI memory written in **Go**. It accepts raw chat logs and files, uses a background worker to automatically **extract** a running summary of each session on ingestion, and lets agents explicitly **distill** sessions into reusable skills that transfer across sessions — all easily searchable.

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
    Agent -- "1. Push Chat Logs and Files" --> API
    API -- "2. Save Messages and File Metadata" --> Postgres
    API -- "3. Upload Assets" --> MinIO
    API -- "4. Dispatch Ingest Jobs" --> Worker
    Worker -- "5. Embed + Extract" --> LLM
    Worker -- "6. Save Embeddings + Tasks" --> Postgres
    
    %% Flow 2: Skill Distillation
    Agent -- "7. Distill Skill" --> API
    API -- "8. Distill Job" --> Worker
    Worker -- "9. Trigger Distillation" --> LLM
    Worker -- "10. Save Skills" --> Postgres

    %% Flow 3: Retrieval (Within a Session)
    Agent -- "11. Get Tasks & Messages Within Session" --> API
    API -- "12. Fetch Session History" --> Postgres
    API -- "13. Return Session History" --> Agent

    %% Flow 4: Retrieval (Across Sessions)
    Agent -- "14. Get Skills & Messages Across Sessions" --> API
    API -- "15. Vector Search" --> Postgres
    API -- "16. Return Relevant Skills/Messages" --> Agent
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
        VECTOR embedding "pgvector(1536)"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    FILE_CHUNKS {
        UUID id PK
        UUID file_id FK
        INT chunk_index
        TEXT content
        VECTOR embedding "pgvector(1536)"
        TIMESTAMPTZ created_at
    }
    
    SPACES ||--o{ SKILLS : ""
    SPACES |o--o{ SESSIONS : ""
    SPACES |o--o{ FILES: ""
    
    SESSIONS ||--o{ TASKS : ""
    TASKS |o--o{ MESSAGES : ""
    SESSIONS ||--o{ MESSAGES : ""

    MESSAGES ||--o{ MESSAGE_ASSETS : ""
    ASSETS ||--o{ MESSAGE_ASSETS : ""

    FILES ||--o{ FILE_CHUNKS : ""
    ASSETS ||--o{ FILES : ""
```