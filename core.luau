--!nonstrict
-- Ember core: turns Claude Code hook events (and, optionally, edge's state)
-- into one `view` that the bar widget and the panel render. It holds pending
-- permission requests and questions and writes the answer the hook waits for.

local art = require("./lib/art.luau")

local DATA = noctalia.pluginDataDir()
local PANEL = "screenager/ember:card"
local HOME = noctalia.expandPath("~")
local ASK_MS = 90000

local sessions = {} -- session_id -> session
local asks = {} -- pending permission requests / questions, oldest first
local flash = nil -- short banner: done, edge notes
local edge = nil -- edge's status table (nil when edge isn't talking to us)
local edgeCards = {} -- ordered edge cards with buttons
local frames, palette = nil, nil
local panelOpen = false
local rev = 0

local function now()
  return noctalia.nowMs()
end

local function project(cwd)
  if not cwd or cwd == "" or cwd == HOME then
    return "~"
  end
  return cwd:match("([^/]+)/*$") or cwd
end

local function base(p)
  return (p or ""):match("([^/]+)$") or (p or "")
end

local function lines(s)
  if not s or s == "" then
    return 0
  end
  local _, n = s:gsub("\n", "")
  return n + 1
end

-- activity turns a tool call into a short present-tense label.
local function activity(tool, input)
  input = input or {}
  local s = function(k) return input[k] or "" end
  if tool == "Read" then
    return "Reading " .. base(s("file_path")), 0, 0
  elseif tool == "Edit" then
    return "Editing " .. base(s("file_path")), lines(s("new_string")), lines(s("old_string"))
  elseif tool == "MultiEdit" then
    local p, m = 0, 0
    for _, e in ipairs(input.edits or {}) do
      p += lines(e.new_string)
      m += lines(e.old_string)
    end
    return "Editing " .. base(s("file_path")), p, m
  elseif tool == "Write" then
    return "Writing " .. base(s("file_path")), lines(s("content")), 0
  elseif tool == "Bash" then
    if s("description") ~= "" then
      return s("description"), 0, 0
    end
    return "Running " .. s("command"), 0, 0
  elseif tool == "Grep" or tool == "Glob" then
    return "Searching " .. s("pattern"), 0, 0
  elseif tool == "WebFetch" then
    return "Reading " .. (s("url"):match("^%a+://([^/]+)") or "the web"), 0, 0
  elseif tool == "WebSearch" then
    return "Searching the web", 0, 0
  elseif tool == "Task" or tool == "Agent" then
    return "Delegating: " .. s("description"), 0, 0
  elseif tool == "Skill" then
    return "Using " .. s("skill"), 0, 0
  elseif tool == "TodoWrite" then
    return "Planning", 0, 0
  elseif tool and tool:sub(1, 5) == "mcp__" then
    return "Using " .. (tool:match("__([^_]+)$") or tool):gsub("_", " "), 0, 0
  end
  return tool or "Working", 0, 0
end

local function alive(pid)
  return pid ~= nil and pid > 1 and noctalia.fileExists("/proc/" .. pid)
end

local function parsePids(s)
  local out = {}
  for p in (s or ""):gmatch("%d+") do
    table.insert(out, tonumber(p))
  end
  return out
end

-- ancestors walks /proc up from Claude to the terminal.
local function ancestors(pids)
  local out, seen = {}, {}
  for _, start in ipairs(pids) do
    local pid = start
    for _ = 1, 10 do
      if not pid or pid <= 1 or seen[pid] then
        break
      end
      seen[pid] = true
      table.insert(out, pid)
      local st = noctalia.readFile("/proc/" .. pid .. "/stat")
      if not st then
        break
      end
      pid = tonumber((st:match("%)%s+%S+%s+(%d+)")))
    end
  end
  return out
end

-- ---- compositor: find and focus a session's terminal ----

local function focusSession(s)
  local chain = ancestors(s.pids)
  if noctalia.getenv("NIRI_SOCKET") then
    noctalia.runAsync({ "niri", "msg", "-j", "windows" }, function(r)
      local wins = noctalia.json.decode(r.stdout or "") or {}
      for _, p in ipairs(chain) do
        for _, w in ipairs(wins) do
          if w.pid == p then
            noctalia.runAsync({ "niri", "msg", "action", "focus-window", "--id", tostring(w.id) })
            return
          end
        end
      end
    end)
  elseif noctalia.getenv("HYPRLAND_INSTANCE_SIGNATURE") then
    noctalia.runAsync({ "hyprctl", "-j", "clients" }, function(r)
      local wins = noctalia.json.decode(r.stdout or "") or {}
      for _, p in ipairs(chain) do
        for _, w in ipairs(wins) do
          if w.pid == p then
            noctalia.runAsync({ "hyprctl", "dispatch", "focuswindow", "pid:" .. p })
            return
          end
        end
      end
    end)
  elseif noctalia.getenv("SWAYSOCK") then
    for _, p in ipairs(chain) do
      noctalia.runAsync({ "swaymsg", "[pid=" .. p .. "]", "focus" })
    end
  end
end

-- focusedIsSession reports (async) whether the session's terminal is focused.
local function focusedIsSession(s, cb)
  if not noctalia.getenv("NIRI_SOCKET") then
    cb(false)
    return
  end
  local chain = ancestors(s.pids)
  noctalia.runAsync({ "niri", "msg", "-j", "focused-window" }, function(r)
    local w = noctalia.json.decode(r.stdout or "")
    for _, p in ipairs(chain) do
      if w and w.pid == p then
        cb(true)
        return
      end
    end
    cb(false)
  end)
end

-- ---- the view ----

local function sessionList()
  local out = {}
  for _, s in pairs(sessions) do
    table.insert(out, s)
  end
  local rank = { wait = 3, work = 2, idle = 1 }
  table.sort(out, function(a, b)
    if rank[a.state] ~= rank[b.state] then
      return rank[a.state] > rank[b.state]
    end
    return a.seen > b.seen
  end)
  return out
end

local function askView(a)
  if a.kind == "perm" then
    local btns = { { key = "deny", label = "Deny", variant = "ghost" } }
    if a.suggestions then
      table.insert(btns, { key = "always", label = "Always", variant = "ghost" })
    end
    table.insert(btns, { key = "allow", label = "Allow", variant = "primary" })
    return {
      kind = "perm", key = "ask:" .. a.id, target = "ask:" .. a.id, mood = "wait",
      text = a.short or a.title, title = a.title, sub = a.project, code = a.code, body = a.body,
      plus = a.plus, minus = a.minus, buttons = btns,
    }
  end
  local q = a.questions[a.index]
  return {
    kind = "ask", key = "ask:" .. a.id .. ":" .. a.index, target = "ask:" .. a.id, mood = "wait",
    text = q.question, sub = a.project, index = a.index, total = #a.questions,
    options = q.options, multi = q.multiSelect, picked = a.picked,
    buttons = { { key = "panel", label = "Answer", variant = "primary" } },
  }
end

local function publish()
  rev += 1
  local list = sessionList()
  local v = { rev = rev, frames = frames, palette = palette, edge = edge, sessions = {} }
  for i, s in ipairs(list) do
    if i > 6 then
      break
    end
    table.insert(v.sessions, { id = s.id, project = s.project, state = s.state, label = s.label, since = s.since })
  end

  local top
  if #asks > 0 then
    top = askView(asks[1])
    top.queued = #asks - 1
  elseif flash then
    top = flash
  elseif #edgeCards > 0 then
    top = edgeCards[1]
  else
    local s = list[1]
    if s and s.state == "wait" then
      top = { kind = "wait", key = "wait:" .. s.id, mood = "wait", text = s.project .. " needs you", target = "focus:" .. s.id }
    elseif s and s.state == "work" then
      local busy = 0
      for _, x in ipairs(list) do
        if x.state == "work" then
          busy += 1
        end
      end
      top = { kind = "work", key = "work:" .. s.id, mood = "work", text = s.label ~= "" and s.label or "Thinking",
        plus = s.plus, minus = s.minus, more = busy - 1, target = "focus:" .. s.id }
    end
  end
  if not top and edge and edge.task and edge.task ~= "" then
    top = { kind = "edge", key = "task:" .. edge.task, mood = edge.pending and "idle" or "work", text = edge.task,
      startedMs = (not edge.pending) and edge.startedMs or nil }
    if edge.pending then
      top.buttons = { { key = "start", label = "Start", variant = "primary" } }
      top.target = "edgebtn"
    end
  end
  v.top = top or { kind = "idle", key = "idle", mood = "idle" }
  noctalia.state.set("view", v)
end

-- ---- answers ----

local function respond(a, out)
  local tmp = a.file .. ".answer.tmp"
  noctalia.writeFile(tmp, out or "")
  noctalia.renameFile(tmp, a.file .. ".answer")
end

local function dropAsk(id)
  for i, a in ipairs(asks) do
    if a.id == id then
      table.remove(asks, i)
      return a
    end
  end
end

local function answerPerm(a, key)
  local decision
  if key == "allow" then
    decision = { behavior = "allow" }
  elseif key == "always" then
    decision = { behavior = "allow", updatedPermissions = a.suggestions }
  elseif key == "deny" then
    decision = { behavior = "deny", message = "Denied from the Noctalia bar." }
  end
  dropAsk(a.id)
  a.session.state = "work"
  if decision then
    respond(a, noctalia.json.encode({ hookSpecificOutput = { hookEventName = "PermissionRequest", decision = decision } }))
  else
    respond(a, "") -- "In terminal": Claude asks there
  end
end

local function answerQuestion(a, key)
  local q = a.questions[a.index]
  if key == "terminal" then
    dropAsk(a.id)
    respond(a, "")
    return
  end
  local idx = tonumber(key:match("^opt:(%d+)$"))
  if idx and q.multiSelect then
    a.picked[idx] = not a.picked[idx] or nil
    return
  end
  local chosen = {}
  if idx then
    chosen = { q.options[idx].label }
  elseif key == "submit" then
    for i, o in ipairs(q.options) do
      if a.picked[i] then
        table.insert(chosen, o.label)
      end
    end
    if #chosen == 0 then
      return
    end
  else
    return
  end
  a.answers[q.question] = table.concat(chosen, ", ")
  if a.index < #a.questions then
    a.index += 1
    a.picked = {}
    return
  end
  dropAsk(a.id)
  a.session.state = "work"
  respond(a, noctalia.json.encode({ hookSpecificOutput = {
    hookEventName = "PreToolUse", permissionDecision = "allow",
    updatedInput = { questions = a.questions, answers = a.answers },
  } }))
  if panelOpen and #asks == 0 then
    noctalia.togglePanel(PANEL)
  end
end

-- ---- hook events ----

local function sessionFor(data, pids)
  local id = data.session_id
  if not id or id == "" then
    return nil
  end
  local s = sessions[id]
  if not s then
    s = { id = id, state = "idle", label = "", plus = 0, minus = 0, since = now(), seen = now(), pids = {} }
    sessions[id] = s
  end
  s.seen = now()
  if data.cwd and data.cwd ~= "" then
    s.project = project(data.cwd)
  end
  s.project = s.project or "~"
  if #pids > 0 then
    s.pids = pids
  end
  return s
end

local function onHook(event, data, pids)
  if event == "SessionEnd" then
    sessions[data.session_id or ""] = nil
    return
  end
  local s = sessionFor(data, pids)
  if not s then
    return
  end
  if event == "UserPromptSubmit" then
    s.state, s.label, s.plus, s.minus, s.since = "work", "Thinking", 0, 0, now()
  elseif event == "PreToolUse" then
    if data.tool_name ~= "AskUserQuestion" then
      s.state = "work"
      s.label, s.plus, s.minus = activity(data.tool_name, data.tool_input)
    end
  elseif event == "PostToolUse" then
    if s.state == "wait" then
      s.state = "work"
    end
  elseif event == "Notification" then
    if data.notification_type == "idle_prompt" then
      s.state = "wait"
    end
  elseif event == "Stop" then
    s.state, s.label = "idle", ""
    local body = (data.last_assistant_message or ""):match("^%s*([^\n]*)") or ""
    body = body:gsub("^[#%*%-%s]+", "")
    focusedIsSession(s, function(focused)
      if focused then
        return -- they're looking at it
      end
      flash = { kind = "done", key = "done:" .. s.id .. ":" .. now(), mood = "happy", text = "Done · " .. s.project,
        body = body, untilMs = now() + 8000, target = "focus:" .. s.id, bounce = true }
      publish()
    end)
  end
end

local function onAsk(event, data, file, hookPid, pids)
  local s = sessionFor(data, pids)
  if not s then
    noctalia.writeFile(file .. ".answer", "")
    return
  end
  s.state = "wait"
  local a = { id = tostring(now()) .. tostring(hookPid), file = file, pid = hookPid, session = s, project = s.project, created = now() }
  local input = data.tool_input or {}
  if event == "AskUserQuestion" then
    a.kind, a.questions, a.index, a.answers, a.picked = "ask", input.questions or {}, 1, {}, {}
    if #a.questions == 0 then
      respond(a, "")
      return
    end
  else
    a.kind = "perm"
    local tool = data.tool_name or "tool"
    local label, p, m = activity(tool, input)
    a.plus, a.minus = p, m
    if tool == "Bash" then
      a.title, a.short = "Run this command?", "Run"
      a.code, a.body = input.command, input.description
    elseif tool == "Edit" or tool == "MultiEdit" or tool == "Write" then
      a.title = (tool == "Write" and "Create " or "Edit ") .. base(input.file_path) .. "?"
      a.body = input.file_path
    elseif tool == "WebFetch" then
      a.title, a.code = "Fetch " .. ((input.url or ""):match("^%a+://([^/]+)") or "page") .. "?", input.url
    else
      a.title = label .. "?"
      a.code = noctalia.json.encode(input)
    end
    local sug = data.permission_suggestions
    if type(sug) == "table" and #sug > 0 then
      a.suggestions = sug
    end
  end
  table.insert(asks, a)
  if a.kind == "ask" and not panelOpen then
    noctalia.togglePanel(PANEL)
  end
end

-- payload: "<Event> <file> <hookPid> <pid,pid>"
local function parse(payload)
  local event, file, hookPid, pids = (payload or ""):match("^(%S+)%s+(%S+)%s+(%d+)%s*(%S*)")
  return event, file, tonumber(hookPid), parsePids(pids)
end

-- ---- edge (optional) ----

local function onEdge(msg)
  if msg.type == "status" then
    edge = msg.status
  elseif msg.type == "card" then
    local c = msg.card
    local btns = {}
    for _, b in ipairs(c.buttons or {}) do
      table.insert(btns, { key = b.key, label = b.label, variant = b.primary and "primary" or "ghost" })
    end
    local card = { kind = "edgecard", key = "edge:" .. c.id .. ":" .. (c.title or ""), target = "edge:" .. c.id,
      id = c.id, mood = c.mood or "work", text = c.title, body = c.body, buttons = btns, bounce = c.bounce }
    for i = #edgeCards, 1, -1 do
      if edgeCards[i].id == c.id then
        table.remove(edgeCards, i)
      end
    end
    if #btns == 0 then
      card.untilMs = now() + 7000
      flash = card
    elseif c.urgent then
      table.insert(edgeCards, 1, card)
    else
      table.insert(edgeCards, card)
    end
  elseif msg.type == "remove" then
    for i = #edgeCards, 1, -1 do
      if edgeCards[i].id == msg.id then
        table.remove(edgeCards, i)
      end
    end
  end
end

-- ---- entry points ----

function onIpc(event, payload)
  if event == "hook" or event == "ask" then
    local ev, file, hookPid, pids = parse(payload)
    if not ev then
      return
    end
    noctalia.readFileAsync(file, function(body)
      local data = body and noctalia.json.decode(body)
      if event == "hook" then
        noctalia.removeFile(file)
        if data then
          onHook(ev, data, pids)
        end
      elseif data then
        onAsk(ev, data, file, hookPid, pids)
      else
        noctalia.writeFile(file .. ".answer", "")
      end
      publish()
    end)
  elseif event == "edge" then
    local msg = noctalia.json.decode(payload or "")
    if msg then
      onEdge(msg)
      publish()
    end
  elseif event == "panel" then
    noctalia.togglePanel(PANEL)
  elseif event == "answer" and asks[1] then
    -- "answer opt:2" / "answer submit" / "answer allow": the oldest request
    local a = asks[1]
    if a.kind == "perm" then
      answerPerm(a, payload or "")
    else
      answerQuestion(a, payload or "")
    end
    publish()
  elseif event == "allow" or event == "deny" or event == "always" or event == "terminal" then
    -- keyboard path: answer the oldest permission request (bind these in niri)
    for _, a in ipairs(asks) do
      if a.kind == "perm" then
        answerPerm(a, event)
        publish()
        return
      end
    end
  end
end

noctalia.state.watch("action", function(a)
  if type(a) ~= "table" then
    return
  end
  local target, key = a.target or "", a.key or ""
  local askId = target:match("^ask:(.+)$")
  if askId then
    for _, x in ipairs(asks) do
      if x.id == askId then
        if key == "panel" then
          if not panelOpen then
            noctalia.togglePanel(PANEL)
          end
        elseif x.kind == "perm" then
          answerPerm(x, key)
        else
          answerQuestion(x, key)
        end
        break
      end
    end
  elseif target:sub(1, 6) == "focus:" then
    local s = sessions[target:sub(7)]
    if s then
      focusSession(s)
    end
    if flash and flash.target == target then
      flash = nil
    end
  elseif target:sub(1, 5) == "edge:" or target == "edgebtn" then
    local id = target:sub(6)
    for i = #edgeCards, 1, -1 do
      if edgeCards[i].id == id then
        table.remove(edgeCards, i)
      end
    end
    if flash and flash.target == target then
      flash = nil
    end
    if key ~= "dismiss" and noctalia.commandExists("edge") then
      noctalia.runAsync({ "edge", "action", key })
    end
  elseif target == "flash" then
    flash = nil
  elseif target == "panel" then
    noctalia.togglePanel(PANEL)
  end
  publish()
end)

noctalia.state.watch("panelOpen", function(v)
  panelOpen = v == true
end)

-- palette, expiry and liveness: once a second
local lastPalette = 0
function update()
  local t, changed = now(), false
  if t - lastPalette > 5000 then
    lastPalette = t
    local dir, key = art.render(DATA)
    if key ~= palette then
      frames, palette, changed = dir, key, true
    end
  end
  if flash and flash.untilMs and t > flash.untilMs then
    flash, changed = nil, true
  end
  for i = #asks, 1, -1 do
    local a = asks[i]
    if not alive(a.pid) or t - a.created > ASK_MS then
      table.remove(asks, i) -- the hook gave up or Claude cancelled
      a.session.state = "work"
      changed = true
    end
  end
  for id, s in pairs(sessions) do
    local live = false
    for _, p in ipairs(s.pids) do
      live = live or alive(p)
    end
    if (#s.pids > 0 and not live) or (s.state == "idle" and t - s.seen > 3 * 3600 * 1000) then
      sessions[id], changed = nil, true
    end
  end
  if changed then
    publish()
  end
end

frames, palette = art.render(DATA)
noctalia.setUpdateInterval(1000)
publish()
