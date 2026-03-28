# x-prozy Context

## Why This Project Exists

The original plan was to heavily refactor `3x-ui`, mainly to get:
- separate client/subscription management
- aggregated subscription traffic
- easier client creation
- node support

During exploration it became clear that:
- `3x-ui` has good UI and protocol-editing ideas
- but its storage/runtime model is too inbound-centric
- node support would force even deeper architectural changes

So `x-prozy` was created as a fresh project that borrows strong ideas from `3x-ui`, while fixing the model from the ground up.

## Reference Projects

### Primary UI / protocol reference

`3x-ui`

Use it for:
- protocol modal UX
- generation helpers
- Reality / TLS / auth forms
- general admin workflow

### Secondary product inspiration

`remnawave`

Use it as a reminder of what not to make painful:
- too much manual JSON-style composition
- awkward node handling
- weak runtime/node visibility

## Core Product Vision

Build a panel and node system for Xray-based infrastructure where:
- clients are first-class entities
- subscriptions are first-class entities
- inbounds are reusable transport definitions
- nodes are first-class remote runtimes
- runtime config is projected from normalized models

## Planned Top-Level Structure

- `panel/backend`
- `panel/web`
- `sub`
- `node`

### panel/backend

Will own:
- auth
- database/domain model
- panel API
- runtime projection
- node control plane
- subscription generation

### panel/web

Will own:
- admin UI
- forms/modals
- dashboard and actions

### sub

Will be a separate subscription-facing UI layer.

Purpose:
- custom subscription page / landing / profile presentation
- not a clone of the full admin panel

### node

Will be the runtime-side component.

Purpose:
- connect to panel
- maintain control channel
- host runtime/Xray side
- report health/status/usage

## Key Architecture Decisions

### 1. Build new, do not keep patching 3x-ui

Reason:
- easier to model nodes correctly
- easier to model clients/subscriptions correctly
- less time spent fighting legacy embedded JSON

### 2. Keep Go as the foundation

Reason:
- good fit for networking, agents, control plane, runtime orchestration
- good process/runtime integration
- matches the strengths already visible in `3x-ui`

ASP.NET was considered attractive for object modeling, but Go remains the current preferred base.

### 3. Clients are not owned by inbound

A client represents:
- username
- subscription identity
- quota/expiry/reset policy
- aggregate usage

Access to inbounds is assigned separately.

### 4. Inbound owns transport behavior, not customer ownership

Inbound represents:
- protocol
- stream settings
- Reality/TLS/auth details
- transport defaults or derived runtime behavior

### 5. Nodes are a core feature

The system should support remote nodes as part of the base architecture.

The earlier design discussion established:
- node should preferably connect outbound to panel
- panel is source of truth
- node should keep working if panel briefly disappears
- liveness should be stream/heartbeat based

## Lessons Learned From 3x-ui Refactor Attempt

### Runtime apply matters

One real bug turned out to be simple:
- client changes existed in storage
- but Xray had not yet applied them
- after restarting Xray, connection worked

Meaning:
- any new client/inbound model must include runtime apply lifecycle
- storage changes alone are not enough

### Flow was a misleading rabbit hole

There was confusion around `flow`.

Important takeaway:
- lack of `flow` was not the root cause of the connection failure in that specific case
- the actual issue was runtime apply/restart

Stronger takeaway:
- derived transport-specific parameters should be handled close to runtime projection
- not blindly as global client properties

## UI Direction

The new panel should keep the strengths of `3x-ui`:
- modular forms
- rich protocol editing
- quick generation buttons
- useful actions near the top of pages
- dashboards/stat blocks where they genuinely help

The earlier attempt at a separate clients page in `3x-ui` was acknowledged as not yet good enough.

For `x-prozy`, use `3x-ui` as the quality bar for:
- layout density
- modal ergonomics
- protocol completeness

## Immediate Development Priority

Current priority:
- define backend/domain direction
- define panel/backend and panel/web structure
- design nodes correctly early

Short-term goal:
- build the new project skeleton and architecture docs first

## Notes For Future Work

### Outbounds

Outbound editing should eventually be more ergonomic than hand-writing JSON.

`3x-ui` outbound editors may be a useful reference here too.

### Subscription UI

`sub` can become a branded/custom UI layer similar in role to `remnawave-sub`, but fully owned by this project.

### MTProxy

Not current priority.

### Cluster/P2P node ideas

Interesting, but not first implementation target.

Start with:
- strong panel/node control plane
- liveness
- reconnect behavior
- runtime projection
