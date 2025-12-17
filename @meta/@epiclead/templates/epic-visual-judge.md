## EPIC INFO ##
$PHASE:     {{PHASE_ID}}
$SPEC_FILE: @meat/@pm/epics/{{EPIC_ID}}_spec.md
$PLAN_FILE: @meat/@pm/epics/{{EPIC_ID}}_plan.md
$EXEC_FILE: @meat/@pm/epics/{{EPIC_ID}}_exec.md


## INSTRUCTIONS ##
YOU are a judge, responsible for evaluating the work of the dev agent.
Read $EPIC_FILE and $PLAN_FILE.

You have to evalute the execution of $PHASE. IF $EXEC_FILE contains more completed phases AFTER $PHASE, evaluate them too.
YOU MUST NOT EVALUATE THE COMPLETENESS OF THE EPIC, ONLY THE WORK ON $PHASE!

Think hard, plan how to work in small incremental steps!
Use SUBAGENTS and protect your context window!

## SPECIAL NOTES
This is a VISUAL epic. You have to check if the web mockups LOOK as expected. 
DEV AGENT STARTED 2 servers: 
- MOCKUPS DEV server in background: `cd kid-mockups/ && bun dev` <- YOU COPY FROM HERE! (port 5173)
- REAL APP DEV SERVER in background: `cd kids-real-ui && bun run dev` <- YOU WORK HERE! (port 3555)

Use playwright mcp to open a browser to http://localhost:5173 (mockups) or http://localhost:3555 (results). 
That way you can visually confirm results of the $PHASE!

## LAST LINES OF OUTPUT ##
Print THE MAGIC WORD: ==REALITY_IS_MAGIC== WHEN DONE
THEN ADD THE VERDICT IN JSON FORMAT!


## EVALUATION ##
- no errors
- not compilation warnings visible
- all the required tests from the $PLAN_FILE are included in tests files
- code is properly structured and maintainable


## SCORING ##

100% -> all test are passing, good quality
90% -> some minor discrepancies, but the code is good, still APPROVE!
85% -> some test are failing, but code is good, RETURN FOR FIXES
75% -> some things are not quite to the standards, RETURN FOR FIXES
70% -> something is really wrong, REJECT THE PHASE


## RESPONSE ##

{"score": 100, "reason": "all good", "decision": "APPROVE"}
{"score": 90, "reason": "some minor discrepancies, all tests passing", "decision": "APPROVE"}
{"score": 85, "reason": "$INSTRUCTIONS TO FIX THE ISSUES, will be sent to the dev agent", "decision": "RETURN_FOR_FIXES"}
{"score": 75, "reason": "$INSTRUCTIONS TO FIX THE ISSUES, will be sent to the dev agent", "decision": "RETURN_FOR_FIXES"}
{"score": 70, "reason": "Bad quality", "decision": "REJECT_THE_PHASE"}
