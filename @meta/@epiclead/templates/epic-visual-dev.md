## EPIC INFO ##
$SPEC_FILE: @meta/@pm/epics/{{EPIC_ID}}_spec.md
$PLAN_FILE: @meta/@pm/epics/{{EPIC_ID}}_plan.md
$EXEC_FILE: @meta/@pm/epics/{{EPIC_ID}}_exec.md
$PHASE_ID: LOOK IN $EXEC_FILE for the NEXT UNCOMPLETED PHASE!


## INFO FILES ##
$GUIDE:     @guide/KID-DESIGN-GUIDE.md
$APP_URLS:  @guide/KID-URL-STRUCTURE.md
$TAILWIND:  @guide/TAILWIND-CONFIG.md


## PROCESS: 

- start MOCKUPS DEV server in background: `cd kid-mockups/ && bun dev` <- YOU COPY FROM HERE! (port 5173)
- START REAL APP DEV SERVER in background: `cd kids-real-ui && bun run dev` <- YOU WORK HERE! (port 3555)
- use playwright mcp to open a browser and compare your results, in unsure! That way you can visually confirm your results! 

After each completed phase: 
- go through Visual Design Validation Scenarios. 
- IF EVERYTHING OK: 
  - Mark your progress in the $EXEC_FILE file!
  - commit your changes!
NO GIT COMMIT Visual Design Validation Scenarios incomplete! RATHER stop and ASK for user input, than continue without correct VALIDATION!



## AUTONOMOUS EXECUTION ##

Execute $PHASE_ID autonomously. Self-correct errors. Submit to judge for review.

**You:** Fast executor (make it work)
**Judge:** Quality control (make it right)

---

## EXECUTION FLOW ##

```
1. Read $SPEC + $PLAN → Understand $PHASE_ID
2. Execute CODE block → Write  → fix → next
3. Execute TESTS block → Write → run → fix → next
5. Mark [X] in $EXEC → Commit → Submit to judge
6. Judge corrections? → Apply → Resubmit
```

---

## ERROR RECOVERY ##

**Strategy: Try 3 times, then fail forward**

**TypeScript error:** Fix 3x → After 3: `@ts-expect-error` + comment → continue
**Test failure:** Debug 3x → After 3: `test.skip()` + TODO → continue
**QA check:** Fix 3x → After 3: Document + continue
**Ambiguous spec:** Check plan/codebase → Choose reasonable → Document → continue

Judge catches if workaround wrong.

---

## DECISION HEURISTICS ##

Priority order:
1. $PLAN says X → Do X
2. Codebase pattern Y → Follow Y
3. TypeScript requires Z → Satisfy Z
4. Multiple options → Simplest

**Never block on:** Naming, style, optimizations, architecture
**Only block on:** Won't compile after 3 tries, missing dependency

---

## JUDGE WORKFLOW ##

```
YOU → Submit → JUDGE → ✅ Approved (DONE)
                    → ❌ Corrections → YOU apply → Resubmit
```

No retry limit with judge - iterate until approved.

---

## COMPLETION CRITERIA ##

Ready for judge when:
- ✅ All CODE/TESTS items done
- ✅ QA passes (or 3 attempts)
- ✅ "Phase Complete When" criteria met
- ✅ Committed with $PHASE_ID

**Focus on:** Tests passing, compiles, follows plan
**Ignore:** Perfect architecture, edge cases not in tests

---

## OUTPUT ##

### Phase Complete:
```
✅ PHASE $PHASE_ID COMPLETED

Files: [list]
Tests: [count, passing]
Notes: [assumptions/workarounds]

✅ PHASE $PHASE_ID COMPLETED
==REALITY_IS_MAGIC==
```

### After Corrections:
```
🔄 PHASE $PHASE_ID - CORRECTIONS APPLIED

Issues: [list]
Fixes: [list]

✅ PHASE $PHASE_ID CORRECTIONS APPLIED
==REALITY_IS_MAGIC==
```

### Blocked:
```
🛑 PHASE $PHASE_ID - BLOCKED

==REALITY_IS_MAGIC==
```

---

## RULES ##

1. **COMPLETE PHASE** - Self-correct, workaround, finish
2. **TESTS PASS** - Non-negotiable (unless skipped)
3. **3 ATTEMPTS MAX** - Then move on
4. **NEVER BLOCK** - Decide, judge corrects
5. **SPEED > PERFECTION** - Judge handles quality

---

## EXAMPLES ##

**Type error:** Check interface → Fix → Continue
**Test fail:** Debug → Fix root cause → Continue
**Ambiguous:** Check patterns → Choose reasonable → Document → Continue

---

**EXECUTE. SELF-CORRECT. COMPLETE. SUBMIT. ITERATE.**