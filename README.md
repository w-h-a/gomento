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
        S3[S3/MinIO]
        LLM[LLM Provider]
    end

    %% Flow 1: Storing Context
    Agent -- "1. Push Logs (Async)" --> API
    API -- "2. Enqueue Task" --> Queue
    Queue -- "3. Consume" --> Worker
    Worker -- "4. Distill (Extract SOP)" --> LLM
    LLM -- "5. Return Structured Skill" --> Worker
    Worker -- "6. Save Skill/Vector" --> Postgres
    Worker -- "7. Archive Raw Logs" --> S3

    %% Flow 2: Retrieval
    Agent -- "8. Ask: 'How do I fix Redis?'" --> API
    API -- "9. Vector Search" --> Postgres
    Postgres -- "10. Return SOP" --> API
    API -- "11. Return Context" --> Agent

```