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
        User[User] <-->|Chat| Agent[Your Agent]
    end

    subgraph "GoMento"
        API[API Gateway]
        Queue[Queue/Channel]
        Worker[Background Worker]
    end

    subgraph "Infrastructure"
        Postgres[Postgres + pgvector]
        LLM[LLM Provider]
    end

    %% Flow 1: Storing Context
    Agent -- "1. Push chat logs" --> API
    API -- "2. Produce Task" --> Queue
    Queue -- "3. Consume Task" --> Worker
    Worker -- "4. Distill (Extract SOP)" --> LLM
    LLM -- "5. Return Structured Skill" --> Worker
    Worker -- "6. Save Skill/Vector" --> Postgres

    %% Flow 2: Retrieval
    Agent -- "7. Ask: 'How do I fix Redis?'" --> API
    API -- "8. Vector Search" --> Postgres
    Postgres -- "9. Return SOP" --> API
    API -- "10. Return Context" --> Agent
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
        TEXT content
        TIMESTAMPTZ created_at
    }

    PROJECTS ||--|{ SPACES : "for every project there are 1 to many spaces"
    PROJECTS ||--|{ SESSIONS : "for every project there are 1 to many sessions"
    
    SPACES ||--|{ SKILLS : "for every space there are 1 to many skills"
    SPACES |o--o{ SESSIONS : "for every session there is 0 or 1 space; for every space there are 0 or more sessions"
    
    SESSIONS ||--|{ MESSAGES : "for every session there are 1 to many messages"
```