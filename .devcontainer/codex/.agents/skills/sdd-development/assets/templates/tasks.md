# Tasks: {{SPEC_NAME}}

## Metadata

- Status: Draft
- Related requirements: {{REQUIREMENTS_OR_BUGFIX_PATH}}
- Related design: {{DESIGN_PATH}}
- Created: {{DATE}}
- Updated: {{DATE}}

## Status notation

- `[ ]` Not started
- `[~]` In progress
- `[x]` Completed
- `[!]` Blocked

## Tasks

- [ ] TASK-001 Add a failing test for the required behavior
  - Requirements: REQ-001
  - Design: Test strategy
  - Depends on: None
  - Files: {{TEST_FILES}}
  - Completion criteria:
    - The test fails for the expected reason before implementation
    - The test directly verifies the corresponding Acceptance Criteria

- [ ] TASK-002 Implement the required behavior
  - Requirements: REQ-001
  - Design: Proposed design
  - Depends on: TASK-001
  - Files: {{IMPLEMENTATION_FILES}}
  - Completion criteria:
    - TASK-001 passes
    - Existing related tests continue to pass

- [ ] TASK-003 Update related documentation
  - Requirements: REQ-001
  - Design: API changes, Data-model changes
  - Depends on: TASK-002
  - Files: {{DOCUMENTATION_FILES}}
  - Completion criteria:
    - Documentation describes the implemented behavior

- [ ] TASK-004 Run regression verification and reconcile the spec
  - Requirements: REQ-001
  - Design: Test strategy
  - Depends on: TASK-002, TASK-003
  - Files: {{SPEC_FILES}}
  - Completion criteria:
    - Required validation commands pass
    - Acceptance Criteria are verified
    - Specification and implementation are consistent

## Traceability

| Requirement | Design | Task | Test |
|---|---|---|---|
| REQ-001 | Proposed design, Test strategy | TASK-001, TASK-002, TASK-004 | {{TEST_NAME}} |
| NFR-001 | Test strategy | TASK-001, TASK-004 | {{NON_FUNCTIONAL_TEST}} |
| COMP-001 | Compatibility and migration | TASK-002, TASK-004 | {{COMPATIBILITY_TEST}} |
| SEC-001 | Security considerations | TASK-001, TASK-002, TASK-004 | {{SECURITY_TEST}} |

## Blockers

- None
