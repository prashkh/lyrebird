# Visual Metaphor Brainstorm — Making the Story Delightful

> Written 2026-04-28 in response to: "the arrow to enter each edit is a bit
> conspicuous… some buttons to restore checkpoint got deleted I think… it
> would be cool to add some delightful card mimicking a book being flipped
> or a scroll bar timer for each action… let's come up with some ideas first."

## The complaints, separated

There are three different things tangled in the feedback:

1. **The hover arrow is too conspicuous.** The "show what changed" entry
   point appears as a `→` arrow on hover. It works mechanically but it's a
   weak affordance — small, only sometimes visible, and not delightful.
2. **Restore buttons feel missing.** Today they only live on the per-file
   history page. The timeline and the snapshot detail page have no way to
   "go back to this state." That's a real UX gap — the user shouldn't have
   to drill into a file to roll back.
3. **The history could feel more delightful.** Right now it's a
   well-organized list. The user wants something that *feels* like time
   passing — a book, a scroll, a timeline scrubber, something visually rich.

Items 1 and 2 are concrete fixes I'll tackle no matter what. Item 3 is the
fun part — picking a metaphor.

## Quick fixes (not debatable, just doing)

- **Make the entry point more confident.** Replace the hover-only arrow
  with a small, always-visible chevron at the right edge of each row. Or:
  the whole row is visibly hoverable with a subtle "card lift" on hover
  and a chevron that's already there at low opacity, gaining weight on
  hover.
- **Add restore buttons in two more places**:
  - On each story item (not on hover — always visible as a quiet icon
    button next to the time): "↶ rewind to here"
  - On the snapshot detail page (`show.html`) as a primary action button
    in the page header: "↶ Bring the folder back to this state"
- **Confirm before destructive ops** (already done) plus show a flash
  message after success: "Restored fib.py — you can undo this from the
  Story page."

## The visual metaphor — six ideas

I'll describe each, sketch in ASCII, and rate on (a) delight, (b)
readability, (c) implementation cost, (d) scalability to many events.

---

### Option A: Book / Chapter

**Concept**: The folder's history is a book. Each day is a chapter.
Subtle paper / cream tones, serif chapter headings, soft drop shadows on
"pages." Optionally a page-flip animation when navigating between days.

```
┌────────────────────────────────────────────────────┐
│                                                    │
│   Chapter 3 · Yesterday                            │
│   ─────────────────────                            │
│                                                    │
│      Late afternoon, Claude added a test for       │
│      the fib function.                             │
│        "Add a test for the fib function..."       │
│                                                    │
│      Earlier, you edited scratch.md.               │
│                                                    │
│      In the morning, you started tracking         │
│      this folder.                                  │
│                                                    │
│                                              p. 3  │
└────────────────────────────────────────────────────┘
```

- **Delight**: ★★★★★ — feels like a journal, very evocative
- **Readability**: ★★★ — prose-y; great for casual browsing, mediocre for
  dense activity
- **Cost**: medium — paper texture, serif typography, day grouping done
- **Scale**: ★★ — falls apart at 100+ events/day

**Best when**: low-activity folder, someone who wants to *read* their
history.

---

### Option B: Vertical timeline with spine

**Concept**: A literal timeline. A vertical line down the page, events
"hanging" off it as nodes. Day labels float on the left edge as you
scroll. Strong rhythm, very easy to scan.

```
   ╔═══ TODAY ═══╗
   ║              ║
   ║      ●━━━━━━━━━ Just now
   ║              ║ ✋ You edited hello.py
   ║              ║
   ║      ●━━━━━━━━━ 5 min ago
   ║              ║ 🤖 Claude · edited fib.py
   ║              ║   "Add a main block..."
   ║              ║
   ╚══ YESTERDAY ╝
   ║              ║
   ║      ●━━━━━━━━━ 7 hours ago
   ║              ║ ✋ You edited scratch.md
```

The dot color reflects the actor. The line dims with age.

- **Delight**: ★★★★ — modern, satisfying, very GitHub/Linear/Vercel
- **Readability**: ★★★★★ — best in class for chronological data
- **Cost**: low — pure CSS, no special assets
- **Scale**: ★★★★★ — works at 5 events or 5000

**Best when**: serious work tool that should still feel modern and clean.

---

### Option C: Time-machine scrubber

**Concept**: A horizontal slider across the top. Drag it left to "rewind
the folder." The page below shows the state of the folder at that moment
— files that existed then, the most recent change, the open conversation.
You're literally moving through time.

```
┌─────────────────────────────────────────────────────────┐
│  ◀ ●━━━━━━━━━━━━━━━━━━━━━━━●━━━━━━━━━━━━━━━━━ ▶  NOW │
│   start                  2:14pm yesterday              │
├─────────────────────────────────────────────────────────┤
│                                                         │
│   The folder at 2:14pm yesterday:                      │
│                                                         │
│     Files: hello.py · notes.md                         │
│                                                         │
│   Most recent change at this moment:                   │
│     ✋ You · edited hello.py · "small fix"             │
│                                                         │
│   Open conversation: (none)                            │
│                                                         │
│   [↶ Bring the folder back to this state]              │
└─────────────────────────────────────────────────────────┘
```

- **Delight**: ★★★★★ — actual time travel, hard to top
- **Readability**: ★★★ — only one moment visible at a time; you can't
  scan all activity at once
- **Cost**: high — needs scrubber UI, virtualized state preview, smooth
  animation
- **Scale**: ★★★★★ — scales to anything, you only render one moment

**Best when**: the user's primary task is "go to a past state and grab
something / restore."

---

### Option D: Polaroid / postcard stack

**Concept**: Each event is a small card with a slight rotation, like a
photo on a corkboard. Newest cards in front. Hover to lift / un-rotate.
Tactile, memory-album feel.

```
            ┌────────────────────┐
           ╱│ ✋ You              │
          ╱ │ edited scratch.md   │
         ╱  │ · 7 hours ago      │
        ╱   └────────────────────┘
       ┌────────────────────┐
      ╱│ 🤖 Claude          │
     ╱ │ edited test_fib.py │
    ╱  │ "Add a test..."    │
   ╱   │ · 7 hours ago      │
  ╱    └────────────────────┘
 ┌────────────────────┐
 │ 🐦 Started tracking │
 │ · 7 hours ago      │
 └────────────────────┘
```

- **Delight**: ★★★★ — tactile, memory-album feel
- **Readability**: ★★★ — rotations slightly hurt scannability
- **Cost**: low–medium — just transforms and shadows
- **Scale**: ★★★ — gets noisy past 30+ items

**Best when**: photo / personal-memories vibe.

---

### Option E: Receipt / printer scroll

**Concept**: A long, narrow continuous strip — like an old printer roll
or a receipt. Monospace headers, day perforations, subtle paper grain.
Implies append-only, immutable.

```
   ╲─────────────────────────────────╱
    ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
   ▏  YESTERDAY · APR 28           ▕
   ▏                                ▕
   ▏  04:12 pm  ✋ You              ▕
   ▏            edited hello.py     ▕
   ▏                                ▕
   ▏  03:48 pm  🤖 Claude           ▕
   ▏            edited fib.py       ▕
   ▏            "Add a main..."     ▕
   ▏                                ▕
    ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
   ╱─────────────────────────────────╲
```

- **Delight**: ★★★★ — strong, distinct identity
- **Readability**: ★★★★ — monospace is dense but very scannable
- **Cost**: medium — needs paper texture, perforations, careful typography
- **Scale**: ★★★★★ — receipts are made for length

**Best when**: developer-y vibe, immutable / append-only feel.

---

### Option F: Subway map (actor lines)

**Concept**: Multiple horizontal lines — one per actor (You, Claude,
Lyrebird). Stations are events. Lines cross at "collaboration moments"
when both edited the same file in the same window. Time flows left → right.

```
                          (here)
   You      ───●─────●──────────────●──────●──→
                │     │              │      │
   Claude   ─────────●──────●─●─────────────→
                                                
   Lyrebird ●───────────────────────────●────→
            init                       restore
```

- **Delight**: ★★★★★ — incredibly evocative, "see how you and Claude
  worked together"
- **Readability**: ★★ — needs a legend; not obvious what stations mean
  without hovering
- **Cost**: high — SVG layout, line routing, hit testing
- **Scale**: ★★★ — gets crowded fast on a busy day

**Best when**: showing collaboration patterns is the goal.

---

## My recommendation as a UX designer

**Primary: Option B (vertical timeline with spine).** Highest readability,
lowest cost, scales perfectly, looks modern. It's the safe, professional
foundation.

**Then layer in delight from Option A (book) at the day level.** Each day
*group* gets a subtle "page" treatment — soft warm tone, a serif chapter
heading like "Chapter 3 · Yesterday", maybe a light page-edge shadow. The
events inside use the timeline spine style. So the macro-structure feels
like flipping through a book of days, while the micro-structure stays
clean and scannable.

**Add Option C (scrubber) as a separate "Time travel" view** behind a
dedicated button, for the use case of "I want to grab something from a
specific past moment and restore it." Don't replace the main view with it.

This gives us:
- A daily-readable story (book pages by day)
- A dense, modern within-day timeline (vertical spine)
- A dedicated time-travel scrubber for restore tasks (separate page)

Plus the quick fixes:
- Always-visible quiet chevron on each story item (no hover surprise)
- "↶ rewind to here" button on each story item (always visible)
- "↶ Bring the folder back to this state" primary action on the snapshot
  detail page

## ASCII mockup of the recommended hybrid

```
   ┌──────────────────────────────────────────────────┐
   │  myfolder                                        │
   │  3 from Claude · 4 from you · across 2…         │
   │  [↶ Undo]  [＋ Save]  [📦 Hand off]  [⏱ Travel] │
   └──────────────────────────────────────────────────┘

   ╭──── Chapter 2 · Yesterday ──────────────╮
   │                                          │
   │    │                                     │
   │    ●  ✋ You · edited hello.py           │
   │    │     · 4:12pm    ↶ rewind   ›       │
   │    │                                     │
   │    ●  🤖 Claude · edited fib.py          │
   │    │     "Add a main block…"             │
   │    │     · 3:48pm    ↶ rewind   ›       │
   │    │                                     │
   │    ●  🐦 Started tracking · 9am          │
   │                                          │
   ╰──────────────────────────────────────────╯

   ╭──── Chapter 1 · Apr 27 ─────────────────╮
   │  …                                       │
   ╰──────────────────────────────────────────╯
```

The chapter cards use a warm cream tone (or in dark mode, a slightly
warmer surface tone). Vertical spine is a 1px line in the dim text color.
Nodes are 8px circles in the actor color. Each row has a quiet chevron
on the right at low opacity, gaining weight on hover, plus a "↶ rewind"
text-button that's always visible but quiet.

## What I'd want to know from you

1. **Vibe**: serious-modern (Option B alone) or delightful-personal
   (B + book chapters)?
2. **Time travel scrubber** as a separate view: yes/no/maybe-later?
3. **Light or dark theme** as the primary design target? (You already
   built both — but which one should I optimize the polish for?)
4. **Any of the other options** (subway map, polaroid, receipt) speak to
   you that I dismissed?
5. **Restore-from-anywhere**: should the per-event "rewind" button be
   always visible (always-on click target) or hover-revealed (cleaner
   default state, slight discoverability cost)?
