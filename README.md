# gomento

## Problem

If your agent solves a complex bug today, and you ask it to solve the same bug next week, it starts from scratch. It doesn't remember the solution; it just has a massive log file that it can't read efficiently.

Agents make the same mistakes repeatedly because their successes are buried in raw logs. They don't learn; they just execute.

## Solution

GoMento is a high-performance, single-binary sidecar written in **Go**. It accepts raw chat logs, uses a background worker to **distill** those logs into SOPs (Standard Operating Procedures), and makes them searchable for your agent next time.

### Usage

Coming soon!

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
    Agent -- "1. Push Chat Logs & Files" --> API
    API -- "2. Upload File" --> MinIO
    MinIO -- "3. Return URL/Key" --> API
    API -- "4. Save Metadata" --> Postgres
    API -- "5. Produce Task" --> Queue
    
    %% Flow 2: Distillation
    Queue -- "6. Consume" --> Worker
    Worker -- "7. Distill (Extract SOP)" --> LLM
    LLM -- "8. Return Structured Skill" --> Worker
    Worker -- "9. Save Skill/Vector" --> Postgres

    %% Flow 3: Retrieval
    Agent -- "10. Ask: 'How do I fix Redis?'" --> API
    API -- "11. Vector Search" --> Postgres
    Postgres -- "12. Return SOP" --> API
    API -- "13. Return Context" --> Agent
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

    MESSAGES {
        UUID id PK
        UUID session_id FK
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

    MESSAGES ||--|{ MESSAGE_ASSETS : ""
    ASSETS ||--|{ MESSAGE_ASSETS : ""
```