--!nonstrict
-- Ember's panel, attached under the bar: answers questions, shows the full
-- permission request, and otherwise lists sessions and edge's day.

local view = noctalia.state.get("view")
local isOpen = false

local function now()
  return noctalia.nowMs()
end

local function act(target, key)
  noctalia.state.set("action", { target = target, key = key, n = now() })
end

local function minutes(ms)
  local m = math.floor(math.max(ms, 0) / 60000)
  if m < 60 then
    return m .. "m"
  end
  return string.format("%dh%02dm", m // 60, m % 60)
end

local function emberImage(mood)
  if not (view and view.frames) then
    return ui.glyph({ name = "flame", size = 20, color = "primary" })
  end
  return ui.image({ key = "ember-" .. mood .. (view.palette or ""), path = view.frames .. "/" .. mood .. "-1.svg", width = 20, height = 24 })
end

local function header(mood, title, meta)
  return ui.row({ gap = 10, align = "center" }, {
    emberImage(mood),
    ui.label({ text = title, fontSize = 14, fontWeight = "semibold", maxLines = 2, flexGrow = 1 }),
    meta and ui.label({ text = meta, fontSize = 12, color = "on_surface_variant" }) or nil,
  })
end

local function questionView(top)
  local rows = {}
  for i, o in ipairs(top.options or {}) do
    local on = top.picked and top.picked[i]
    local kids = {}
    if top.multi then
      table.insert(kids, ui.glyph({ name = on and "square-check" or "square", size = 16, color = on and "primary" or "on_surface_variant" }))
    end
    table.insert(kids, ui.label({ text = o.label, fontSize = 13, fontWeight = "medium" }))
    if o.description and o.description ~= "" then
      table.insert(kids, ui.label({ text = o.description, fontSize = 12, color = "on_surface_variant", maxLines = 1, flexGrow = 1 }))
    end
    table.insert(rows, ui.row({
      key = "opt" .. i, gap = 10, align = "center", paddingH = 12, paddingV = 9, radius = 10,
      fill = on and "primary/0.14" or "surface_variant/0.7",
      onClick = function() act(top.target, "opt:" .. i) end,
    }, kids))
  end
  local meta = top.sub
  if (top.total or 1) > 1 then
    meta = string.format("%s · %d/%d", top.sub, top.index, top.total)
  end
  local footer = {
    ui.button({ text = "Reply in terminal", variant = "ghost", controlSize = "sm", onClick = function() act(top.target, "terminal") end }),
    ui.spacer({ flexGrow = 1 }),
  }
  if top.multi then
    table.insert(footer, ui.button({ text = "Submit", variant = "primary", controlSize = "sm", onClick = function() act(top.target, "submit") end }))
  end
  return ui.column({ gap = 12, padding = 16 }, {
    header("wait", top.text, meta),
    ui.column({ gap = 6 }, rows),
    ui.spacer({ flexGrow = 1 }),
    ui.row({ gap = 8, align = "center" }, footer),
  })
end

local function permissionView(top)
  local kids = { header("wait", top.title or top.text, top.sub) }
  if top.body and top.body ~= "" then
    table.insert(kids, ui.label({ text = top.body, fontSize = 13, color = "on_surface_variant", maxLines = 2 }))
  end
  if top.code and top.code ~= "" then
    table.insert(kids, ui.column({ padding = 10, radius = 9, fill = "surface_variant/0.8" }, {
      ui.label({ text = top.code, fontFamily = "monospace", fontSize = 12, maxLines = 5 }),
    }))
  end
  if (top.plus or 0) + (top.minus or 0) > 0 then
    table.insert(kids, ui.row({ gap = 8 }, {
      ui.label({ text = "+" .. (top.plus or 0), color = "primary", fontSize = 12 }),
      ui.label({ text = "−" .. (top.minus or 0), color = "error", fontSize = 12 }),
    }))
  end
  table.insert(kids, ui.spacer({ flexGrow = 1 }))
  local row = {
    ui.button({ text = "In terminal", variant = "ghost", controlSize = "sm", onClick = function() act(top.target, "terminal") end }),
    ui.spacer({ flexGrow = 1 }),
  }
  for _, b in ipairs(top.buttons or {}) do
    local variant = b.key == "deny" and "destructive" or (b.key == "always" and "outline" or b.variant)
    table.insert(row, ui.button({ text = b.label, variant = variant, controlSize = "sm", onClick = function() act(top.target, b.key) end }))
  end
  table.insert(kids, ui.row({ gap = 8, align = "center" }, row))
  return ui.column({ gap = 10, padding = 16 }, kids)
end

local function overview()
  local t = now()
  local e = view and view.edge
  local meta = nil
  if e then
    meta = string.format("today %d/%d · streak %d", e.today or 0, e.need or 0, e.streak or 0)
  end
  local kids = { header((view and view.top and view.top.mood) or "idle", "Claude", meta) }
  if e and e.closed then
    table.insert(kids, ui.row({ gap = 6, align = "center" }, {
      ui.glyph({ name = "lock", size = 14, color = "secondary" }),
      ui.label({ text = "Claude is locked outside practice until today's problem is done.", fontSize = 12, color = "on_surface_variant", maxLines = 2 }),
    }))
  end
  if e and e.task and e.task ~= "" then
    local right = e.pending
        and ui.button({ text = "Start", variant = "primary", controlSize = "sm", onClick = function() act("edgebtn", "start") end })
      or ui.label({ text = minutes(t - (e.startedMs or t)), color = "primary", fontSize = 12 })
    table.insert(kids, ui.row({ gap = 10, align = "center", paddingH = 12, paddingV = 6, radius = 10, fill = "surface_variant/0.7" }, {
      ui.glyph({ name = "code", size = 14, color = "primary" }),
      ui.label({ text = e.task, fontSize = 13, fontWeight = "medium", maxLines = 1, flexGrow = 1 }),
      right,
    }))
  end
  local rows = {}
  for _, s in ipairs((view and view.sessions) or {}) do
    local dot = ({ work = "primary", wait = "tertiary" })[s.state] or "outline"
    local label = s.state == "wait" and "waiting for you" or (s.state == "idle" and "idle" or s.label)
    table.insert(rows, ui.row({
      key = s.id, gap = 10, align = "center", paddingH = 12, paddingV = 7, radius = 10,
      onClick = function() act("focus:" .. s.id, "focus") end,
    }, {
      ui.box({ width = 8, height = 8, radius = 4, fill = dot }),
      ui.label({ text = s.project, fontSize = 13, fontWeight = "semibold" }),
      ui.label({ text = label or "", fontSize = 12, color = "on_surface_variant", maxLines = 1, flexGrow = 1 }),
      ui.label({ text = minutes(t - (s.since or t)), fontSize = 12, color = "outline" }),
    }))
  end
  if #rows == 0 then
    table.insert(kids, ui.label({ text = "No Claude Code sessions", fontSize = 12, color = "outline" }))
  else
    table.insert(kids, ui.column({ gap = 2 }, rows))
  end
  return ui.column({ gap = 10, padding = 16 }, kids)
end

local function render()
  if not isOpen then
    return
  end
  local top = view and view.top
  if top and top.kind == "ask" then
    panel.render(questionView(top))
  elseif top and top.kind == "perm" then
    panel.render(permissionView(top))
  else
    panel.render(overview())
  end
end

noctalia.state.watch("view", function(v)
  view = v
  render()
end)

function onOpen()
  isOpen = true
  noctalia.state.set("panelOpen", true)
  panel.setWantsSecondTicks(true)
  render()
end

function onClose()
  isOpen = false
  noctalia.state.set("panelOpen", false)
end

function update()
  render() -- minute counters
end
