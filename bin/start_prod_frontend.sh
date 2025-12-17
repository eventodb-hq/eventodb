export BACKEND_URL=http://macstudio-roman.lemur-bee.ts.net:3333
export NODE_ENV=production
export PORT=9999
cd kids-real-ui
bun run build
bun run start:remote
