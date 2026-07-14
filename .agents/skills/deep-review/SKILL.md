---
name: deep-review
description: Break a finalized implementation plan into independently reviewable vertical slices (Traycer-style), explore the current codebase to ground each slice in what actually exists, then loop each slice through an independent agent whose job is to hunt for weaknesses, risks, and issues the slice would introduce and turn those into a scoped code-change plan — continuing slice by slice until the whole plan is covered. Use whenever the user hands over a finalized plan, spec, or PRD and wants it sliced up, checked against the real codebase, and independently scrutinized for problems before anyone starts coding — including mentions of "Traycer", "slice this plan", "break into slices", "go slice by slice", "find issues with this plan", or "review each slice independently". Also trigger when the user wants mis-sized slices flagged to them rather than silently forced through.
---

# Deep Review

A finalized plan is a promise, not yet a set of changes anyone can safely make, and a promise checked only against itself will miss whatever the codebase actually does differently. This skill turns the plan into a queue of **slices** — small, vertical, independently reviewable units of work — grounds each one in the real code, and runs it through a second, independent pair of eyes whose whole job is to find what's wrong with it before anyone builds it. The point of the loop isn't to produce paperwork; it's to surface weaknesses — wrong assumptions, missed edge cases, code that doesn't work the way the plan assumes it does — while they're still cheap to fix.

Use it once a plan has actually been finalized (not while it's still being negotiated); slicing a moving target just produces slices that don't match reality once the plan settles.

## The loop at a glance

1. **Confirm the plan is finalized** and fully in view.
2. **Explore the codebase** the plan will land in.
3. **Cut the plan into slices**, grounded in what you found.
4. **Size-check every slice** — flag anything too small or too big to the user instead of forcing it through.
5. **Discover reviewer skills** — any skill whose name or description contains "reviewer".
6. **Loop the slices** — for each one, delegate to an independent agent whose job is to find weaknesses, then turn them into a code-change plan.
7. **Report** the consolidated result: what's ready, what's escalated, what's left.

Each stage below ends on a completion criterion — the thing that tells you the stage is actually done, not just started.

## Stage 1 — Confirm the plan is finalized

Read the whole plan before touching it. If it lives in a doc, ticket, or earlier part of the conversation, pull the full text in — don't slice from a partial read or someone's summary of it.

A plan counts as finalized when it states, for every piece of work, what changes, why, and roughly where. If you find open questions, "TBD"s, or requirements that contradict each other, stop here and ask the user to resolve them first. Slicing an unfinished plan produces slices that break the moment the plan settles, and that rework costs more than the pause does now.

**Done when:** you can restate the plan's full scope in your own words, and nothing in it is still being decided.

## Stage 2 — Explore the codebase

A plan is written by someone reasoning about the codebase from memory or from a distance; it's frequently wrong about small things — a function that was renamed, a pattern the codebase actually uses instead of the one assumed, a dependency that isn't there, a component that already handles part of what the plan proposes to build. None of that is visible from the plan text alone.

Before cutting anything, look at the current directory and the parts of the codebase the plan touches: the overall structure, the relevant files and modules, existing conventions (naming, error handling, test patterns, how similar features were built before), and anything the plan references directly (a function, a class, an API, a config). You're not doing a full audit of the repository — you're building enough of a mental map that when you cut and later scrutinize slices, you're checking them against what's actually there instead of against the plan's own description of itself.

Note anything that already contradicts the plan's assumptions (a file that doesn't exist where the plan expects it, a pattern the plan proposes to introduce that conflicts with an established one) — this feeds directly into slicing and later into what the reviewing agents should specifically go looking for.

**Done when:** you can point to the actual files, modules, and conventions each part of the plan will interact with, and you've noted any place the codebase already disagrees with what the plan assumes.

## Stage 3 — Cut the plan into slices

A slice is a **vertical** cut through the plan: one coherent piece of behavior, end to end, that can be built, reviewed, and reasoned about on its own — not a horizontal layer like "all the database changes" or "all the UI changes." Treat each slice as something you could hand to one engineer and one reviewer to finish without needing to understand the rest of the plan in depth — only how their slice connects to it.

For each slice, capture:
- **Name** — short and specific, describing the behavior it delivers
- **Goal** — what becomes true once it's done
- **Touched surface** — the actual files, modules, or components from Stage 2 it's expected to reach into, not a guess from the plan text alone
- **Dependencies** — which other slices need to land first, if any

Order the slices by dependency, not by convenience. A slice that depends on another can't be meaningfully reviewed in isolation until its dependency's change plan exists.

**Done when:** every requirement and behavior in the plan is covered by exactly one slice — no gaps, no overlaps — each slice's touched surface points at real places in the codebase, and the list is ordered so each slice's dependencies precede it.

## Stage 4 — Size-check every slice

Before any slice enters the loop, weigh it against these two failure directions:

- **Too small** — the slice is a single trivial edit with no real surface to review (a rename, a one-line config flag, a stray fix folded in for no reason). It doesn't earn an independent scrutiny pass on its own; that's usually a sign it should be folded into a neighboring slice.
- **Too big** — the slice can't be described in a couple of sentences without an "and also," it spans layers that don't share a reason to change together, or a reviewer would need to hold more than one mental model in their head to scrutinize it properly. That's usually a sign it's actually two or more slices wearing a trenchcoat.

If a slice fails either check, don't quietly split or merge it yourself and move on. Raise it to the user by name, explain which direction it fails in and why, and propose a specific fix — which slices to merge, or how you'd split it — then wait for their call before it enters the loop. The user set the plan's scope; resizing a slice changes that scope, so it's their decision, not a judgment call you make silently on their behalf.

**Done when:** every slice either passes both checks, or has been raised to the user with a named problem and a proposed fix.

## Stage 5 — Discover reviewer skills

Before looping, check which skills are currently available to you and collect every one whose name or description contains "reviewer" (case-insensitive — "code-reviewer," "pr-reviewer," "architecture-reviewer," anything that matches). These are what the scrutinizing agent in Stage 6 is expected to draw on. A slice review that ignores reviewer skills already built for this environment is redoing work that already has a home.

If none exist, say so plainly rather than silently skipping the step — the user may want to know no reviewer skills are installed before the loop starts.

**Done when:** you have a concrete list — possibly empty — of reviewer-keyword skills, ready to hand to each delegated agent.

## Stage 6 — Loop the slices

For each slice, in dependency order:

1. **Delegate it to an independent agent whose job is to find what's wrong with it.** The independence matters: the same agent that just cut the slice tends to defend its own reasoning rather than attack it. If subagents are available (a `Task` tool or equivalent), spawn one per slice. If they aren't, get as close to independence as the environment allows — set aside the reasoning used to cut the slice, re-read only the slice's own description and goal as if seeing it for the first time, and go in looking for problems rather than confirming it's fine.

   Brief the delegated agent with:
   - The slice's name, goal, touched surface, and dependencies
   - The relevant portion of the finalized plan — not the whole plan, unless the slice genuinely needs it
   - Explicit instruction to explore the actual codebase itself (the current directory, the specific files and modules in the slice's touched surface, related tests, callers, and conventions) rather than trusting the slice's description of what's there — the plan and Stage 2's notes are a starting point, not a substitute for looking
   - The reviewer skills discovered in Stage 5, with instruction to actually use them, not just note that they exist
   - The primary ask, stated as what it is: **find weaknesses and issues**, not rubber-stamp the slice. Concretely: wrong assumptions about the code, missed edge cases, conflicts with other slices or with existing behavior, things the plan doesn't account for. Once real issues are surfaced, turn them into a **code change plan** covering:
     - **Weaknesses found** — the specific problems, each pointing at real code, not hypotheticals
     - **Impact** — what breaks, what depends on this, what tests or callers are affected
     - **Changes needed** — the specific files, functions, or components to touch and what changes in each, including fixes for whatever weaknesses were found
     - **Risks or open questions** that remain even after the plan accounts for the known weaknesses

2. **Record the result** against the slice: the weaknesses found, the code change plan, and any concerns that don't have a clean fix yet.

3. **Handle escalations inline.** If the delegated agent reports the slice is actually too small, too big, blocked, or in conflict with another slice's plan, treat that the same way as Stage 4: surface it to the user with specifics rather than resolving it yourself. Pause that slice, keep looping the others that aren't affected, and return to it once the user weighs in.

4. **Move to the next slice** once the current one has either a recorded code change plan or a recorded escalation.

**Done when:** every slice in the ordered list has either a recorded, scrutinized code change plan (with weaknesses either resolved in the plan or explicitly called out as open) or a recorded escalation waiting on the user — none are left in an unknown state.

## Stage 7 — Report

Close the loop with one consolidated view, not a scattered trail of updates. For each slice, show its status (planned or escalated), the weaknesses its reviewer found, its code change plan if it has one, and the impact and risks that remain. Group escalations together at the top so the user sees what needs their decision before anything else.

**Done when:** the user can look at a single summary and know, for every slice, either what will be built and what weaknesses were already caught and fixed, or exactly what decision you're waiting on from them.