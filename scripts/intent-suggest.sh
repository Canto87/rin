#!/bin/bash
# intent-suggest.sh: lightweight skill suggestion from user prompt
# Runs on UserPromptSubmit hook — pattern matching only, no LLM call (<100ms)
set -euo pipefail

INPUT=$(cat)
PROMPT=$(echo "$INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('user_prompt',''))" 2>/dev/null || echo "")

if [ -z "$PROMPT" ]; then
  echo '{"continue":true}'
  exit 0
fi

LOWER=$(echo "$PROMPT" | tr '[:upper:]' '[:lower:]')
SUGGESTION=""

# Skip if user already invoked a skill explicitly
if echo "$LOWER" | grep -qE '^/'; then
  echo '{"continue":true}'
  exit 0
fi

case "$LOWER" in
  *"debug"*|*"error"*|*"crash"*|*"traceback"*|*"디버그"*|*"에러"*|*"왜 안"*|*"안 돼"*)
    SUGGESTION="/troubleshoot" ;;
  *"brainstorm"*|*"ideate"*|*"아이디어"*|*"브레인스토밍"*|*"발산"*)
    SUGGESTION="/ideate" ;;
  *"implement phase"*|*"구현 phase"*|*"run phase"*|*"페이즈"*|*"auto-impl"*)
    SUGGESTION="/auto-impl" ;;
  *"design"*|*"plan"*|*"설계"*|*"기획"*)
    SUGGESTION="/plan-feature" ;;
  *"review"*|*"리뷰"*|*"코드 리뷰"*)
    SUGGESTION="/code-review" ;;
  *"optimize"*|*"experiment"*|*"최적화"*|*"실험"*)
    SUGGESTION="/auto-research" ;;
  *"clean"*|*"entropy"*|*"정리"*|*"gc"*)
    SUGGESTION="/gc" ;;
esac

if [ -n "$SUGGESTION" ]; then
  printf '{"continue":true,"message":"[RIN] Consider using %s for structured execution."}\n' "$SUGGESTION"
else
  echo '{"continue":true}'
fi
