# x-prozy Agent Guide

## Project Intent

`x-prozy` is a new project inspired by `3x-ui`, but not a direct continuation of its legacy data model.

The goal is to keep what is good in `3x-ui`:
- fast Go backend
- practical Xray-oriented admin UX
- rich modal-based protocol editors
- generation helpers for UUID, passwords, keys, Reality values, subscription IDs

And replace what is limiting:
- inbound-centric client model
- weak separation between control plane and data plane
- local-single-node assumptions
- runtime behavior coupled too tightly to panel storage

## Current Direction

Build a new system instead of continuing heavy surgery on `3x-ui`.

Reason:
- `3x-ui` is a strong UI and protocol reference
- but its internal model is too tied to `inbounds.settings.clients`
- nodes, subscriptions, aggregated traffic, and control-plane logic are easier to design cleanly from scratch

## High-Level Modules

Planned structure:
- `panel/backend`: API, auth, storage, runtime orchestration, node control plane
- `panel/web`: admin UI
- `sub`: standalone subscription UI layer
- `node`: remote node agent/runtime side

## Architecture Principles

### 1. Client-first model

Client is a primary entity.

A client owns:
- username
- subscription id
- quota / reset / expiry policy
- aggregated traffic policy
- access to one or more connections/inbounds

Inbound is not the source of truth for a client.

### 2. Inbound as transport definition

Inbound represents:
- protocol
- port/listen
- stream settings
- TLS/Reality/auth settings
- transport-specific defaults

Inbound should inject a runtime user/client projection, not store business ownership.

### 3. Node-aware system

Panel is control plane.
Node is data plane.

Panel should not assume local Xray only.

### 4. Runtime projection

Runtime Xray config must be built from normalized entities:
- clients
- accesses
- inbounds
- nodes

Not from ad-hoc JSON as the primary model.

## What To Reuse From 3x-ui

Reuse concepts and UI behavior, not the whole architecture.

### Strong references

Use `3x-ui` as reference for:
- inbound modal layout
- modular protocol forms
- generation helpers
- Reality/VLESS/TLS editors
- QR / link generation UX
- quick actions like save / restart / copy / regenerate

### Especially useful in 3x-ui

Look at these areas when recreating features:
- protocol forms under `web/html/form`
- modal behavior under `web/html/modals`
- JS models under `web/assets/js/model`
- inbound runtime/update lifecycle under `web/service/inbound.go`
- Xray config generation under `web/service/xray.go`

## What Not To Copy As-Is

Do not reproduce these legacy assumptions:
- clients live inside `inbounds.settings.clients`
- traffic truth comes from per-inbound embedded users
- panel assumes local runtime only
- UI edits are allowed to diverge from runtime apply behavior

## Important Product Decisions

### Clients

Client must be separate from inbound visually and logically.

### Traffic

Traffic model must support both:
- aggregate traffic by subscription/client
- breakdown by connection/inbound

Access decisions use aggregate traffic, not per-inbound totals.

### Flow

`flow` should not be a global client field.

If needed, it should be derived when projecting the client into a compatible inbound/runtime context.

### Nodes

Node support is a first-class concern, not an afterthought.

Prefer:
- node -> panel persistent control channel
- heartbeat/stream-based liveness
- self-registration/enrollment

## Recommended Implementation Order

1. Define core domain model for panel backend
2. Define node model and control-plane contracts
3. Define inbound/connection model
4. Define client/subscription/access model
5. Implement runtime projection layer
6. Rebuild `3x-ui`-style protocol UI on top of the new model
7. Add standalone sub UI

## Rules For Future Agents

1. Treat `3x-ui` as a UX/protocol reference, not as architecture to preserve.
2. Prefer normalized entities over embedded JSON ownership.
3. Before adding UI, define the ownership of data in backend first.
4. Any client/inbound change must consider runtime apply behavior.
5. Any node design must preserve panel as source of truth, while allowing node reconnect and offline survival.
6. When copying ideas from `3x-ui`, record exactly which file/behavior is being borrowed.

## Immediate Focus

Current focus:
- backend/domain design
- panel UI structure
- node architecture

Not current focus:
- full outbound editor parity
- MTProxy
- advanced cluster gossip
