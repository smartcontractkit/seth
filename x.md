```mermaid
flowchart TD
    CEO-->CTO
    CTO-->Engineering
    Engineering-->Research
    Engineering-->Development
    Engineering-->Infra
    Engineering-->Productivity
    Productivity-->CI/CD
    Productivity-->QA
    Productivity-->TT[Test Tooling]
    TT-->Adam
```