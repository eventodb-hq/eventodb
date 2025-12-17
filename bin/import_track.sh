#!/bin/bash
export TOKEN=$(curl -s -X POST "http://localhost:3333/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email": "roman", "password": "password"}' | jq -r '.token')

curl -X POST "http://localhost:3333/api/admin/import-track" \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@/Users/roman/Desktop/work/prj.memomoo/moo_courses_2/tmp/exports/app_019accef-e597-7082-9768-726dea6c6db4.jsonl"

curl -X POST "http://localhost:3333/api/admin/import-track" \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@/Users/roman/Desktop/work/prj.memomoo/moo_courses_2/tmp/exports/app_019acd0d-b00a-78eb-9101-c1ace708b233.jsonl"


curl -X POST "http://localhost:3333/api/admin/import-track" \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@/Users/roman/Desktop/work/prj.memomoo/moo_courses_2/tmp/exports/app_019acd0d-b011-795f-bee4-903bbd2cd957.jsonl"
