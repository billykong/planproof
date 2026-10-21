import Planproof

/-- `planproof-lean` (PRD L-20): one JSON object per input line
(`{"snapshot", "plan", "policy"}`, inputs the Go kernel accepted), one
summary per output line. -/
partial def loop (stdin : IO.FS.Stream) (stdout : IO.FS.Stream) : IO Unit := do
  let line ← stdin.getLine
  if line.isEmpty then return
  let line := line.trimRight
  if !line.isEmpty then
    stdout.putStrLn (Planproof.Json.runLine line)
    stdout.flush
  loop stdin stdout

def main : IO Unit := do loop (← IO.getStdin) (← IO.getStdout)
