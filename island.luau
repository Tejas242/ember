--!nonstrict
-- Ember's bar widget: a pill that morphs with what Claude is doing.
--
-- A change of *identity* (another session, a permission request, "done")
-- collapses the text, swaps it, and springs it open again. A change within
-- the same identity (the next file Claude edits) just eases the width.
-- Ember's frames are stacked in a row with a negative gap and switched by
-- opacity, so animating never re-decodes an image.
--
-- Cost: each barWidget.render makes Noctalia re-lay-out the bar (~2.6 ms on a
-- Ryzen 3 3250U), so identical frames are skipped and ticks run at 16 ms only
-- while the pill morphs, 400 ms while Ember flickers, ~1 s otherwise.

local art = require("./lib/art.luau")

local EW, EH = 15, 18 -- Ember's size in the bar
local FAST, FLICKER, BREATH, SLOW = 16, 400, 900, 1000
local OUT_MS, IN_MS = 130, 320

local view = noctalia.state.get("view")
local shown = nil -- the `top` being displayed
local phase, phaseAt = "rest", 0 -- out → in → rest
local width, widthFrom, widthTo, widthAt = 0, 0, 0, 0
local blinkAt = noctalia.nowMs() + 3000
local interval = 0

local function now()
  return noctalia.nowMs()
end

local function clamp01(x)
  return math.clamp(x, 0, 1)
end

local function easeOut(x)
  x = clamp01(x)
  return 1 - (1 - x) ^ 3
end

local function backOut(x) -- springy arrival, a little overshoot
  x = clamp01(x)
  local c = 1.4
  return 1 + (c + 1) * (x - 1) ^ 3 + c * (x - 1) ^ 2
end

-- est guesses a label's width; it only caps maxWidth, so a generous guess is fine.
local function est(text, size)
  return (utf8.len(text or "") or #(text or "")) * size * 0.58 + 6
end

local function contentWidth(top)
  if not top or top.kind == "idle" then
    return 0
  end
  local w = est(top.text, 12.5)
  if top.code and top.code ~= "" then
    w += math.min(est(top.code, 11.5), 220) + 6
  end
  return math.min(w, 340)
end

local function setInterval(ms)
  if ms ~= interval then
    interval = ms
    noctalia.setUpdateInterval(ms)
  end
end

local function act(target, key)
  noctalia.state.set("action", { target = target, key = key, n = now() })
end

local function retarget(w)
  widthFrom, widthTo, widthAt = width, w, now()
end

local function onView(v)
  view = v
  local top = v and v.top
  if not shown then
    shown = top
    width = contentWidth(top)
    retarget(width)
    return
  end
  if top and top.key == shown.key then
    shown = top -- same identity: ease to the new width
    retarget(contentWidth(top))
  elseif phase ~= "out" then
    phase, phaseAt = "out", now() -- collapse first; swap at the bottom
    retarget(0)
  end
  setInterval(FAST)
end

noctalia.state.watch("view", onView)

-- ---- Ember ----

local function frameIndex(mood, t)
  local frames = art.moods[mood].frames
  local n = #frames
  local blinking = t >= blinkAt and t < blinkAt + 140
  if t >= blinkAt + 140 then
    blinkAt = t + 3200 + math.random(0, 2400)
  end
  if mood == "work" then
    return blinking and n or (math.floor(t / FLICKER) % (n - 1)) + 1
  elseif mood == "wait" then
    return blinking and n or (math.floor(t / BREATH) % 2) + 1
  elseif mood == "happy" then
    return 1
  end
  return blinking and n or 1
end

local function ember(mood, t)
  local dir = view and view.frames
  if not dir then
    return ui.glyph({ key = "glyph", name = "flame", size = 15, color = "primary" })
  end
  mood = art.moods[mood] and mood or "idle"
  local cur = frameIndex(mood, t)
  local layers = {}
  for i = 1, #art.moods[mood].frames do
    layers[i] = ui.image({
      key = mood .. i .. (view.palette or ""),
      path = string.format("%s/%s-%d.svg", dir, mood, i),
      width = EW, height = EH,
      opacity = i == cur and 1 or 0,
    })
  end
  return ui.row({ key = "ember", gap = -EW, width = EW, height = EH, align = "center" }, layers)
end

-- ---- render ----

local function minutes(ms)
  local m = math.floor(ms / 60000)
  if m < 60 then
    return m .. "m"
  end
  return string.format("%dh%02dm", m // 60, m % 60)
end

local lastSig = ""

local function render(t)
  local top = shown or { kind = "idle", key = "idle", mood = "idle" }
  local open = width > 1
  local reveal = (phase == "rest") and 1 or (phase == "in" and easeOut((t - phaseAt) / IN_MS) or 0)
  -- Skip identical frames: every render makes the bar re-lay-out and repaint.
  local e0 = view and view.edge
  local mood = (art.moods[top.mood or "idle"] and top.mood) or "idle"
  local sig = table.concat({ view and view.rev or 0, top.key or "", top.text or "", frameIndex(mood, t), math.floor(width),
    math.floor(reveal * 20), top.startedMs and math.floor((t - top.startedMs) / 60000) or -1, tostring(e0 and e0.closed),
    view and view.palette or "" }, "|")
  if sig == lastSig then
    return
  end
  lastSig = sig
  local children = { ember(top.mood or "idle", t) }

  if open then
    table.insert(children, ui.label({
      key = "text", text = top.text or "", maxWidth = math.max(width, 1), maxLines = 1,
      fontSize = 12.5, fontWeight = "medium", color = top.mood == "wait" and "tertiary" or "on_surface",
    }))
    local rest = width - est(top.text, 12.5)
    if top.code and top.code ~= "" and rest > 24 then
      table.insert(children, ui.label({
        key = "code", text = top.code, maxWidth = rest - 6, maxLines = 1,
        fontSize = 11.5, fontFamily = "monospace", color = "on_surface_variant",
      }))
    end
    if (top.plus or 0) > 0 then
      table.insert(children, ui.label({ key = "plus", text = "+" .. top.plus, fontSize = 12, color = "primary", opacity = reveal }))
    end
    if (top.minus or 0) > 0 then
      table.insert(children, ui.label({ key = "minus", text = "−" .. top.minus, fontSize = 12, color = "error", opacity = reveal }))
    end
    if (top.more or 0) > 0 then
      table.insert(children, ui.label({ key = "more", text = "+" .. top.more, fontSize = 11, color = "on_surface_variant", opacity = reveal }))
    end
    if top.queued and top.queued > 0 then
      table.insert(children, ui.label({ key = "queued", text = "1/" .. (top.queued + 1), fontSize = 11, color = "on_surface_variant", opacity = reveal }))
    end
    if top.startedMs then
      table.insert(children, ui.label({ key = "clock", text = minutes(t - top.startedMs), fontSize = 12, color = "primary", opacity = reveal }))
    end
    for _, b in ipairs(top.buttons or {}) do
      table.insert(children, ui.button({
        key = "b-" .. b.key, text = b.label, variant = b.variant or "ghost", height = 20, fontSize = 12,
        opacity = reveal, enabled = reveal > 0.5,
        onClick = function() act(top.target or "", b.key) end,
      }))
    end
  end

  -- edge's gate, when it's closed and nothing else says so
  local e = view and view.edge
  if e and e.closed and top.kind ~= "edge" and top.kind ~= "edgecard" then
    table.insert(children, ui.glyph({ key = "lock", name = "lock", size = 12, color = "secondary" }))
  end

  local active = open and top.kind ~= "idle"
  barWidget.render(ui.row({
    key = "pill", gap = 6, align = "center", paddingH = active and 8 or 2, radius = 10,
    fill = active and string.format("surface_variant/%.2f", 0.85 * math.min(1, width / 40)) or nil,
  }, children))
end

function update()
  local t = now()
  -- morph phases
  if phase == "out" and t - phaseAt >= OUT_MS then
    shown = view and view.top or shown
    phase, phaseAt = "in", t
    retarget(contentWidth(shown))
  elseif phase == "in" and t - phaseAt >= IN_MS then
    phase = "rest"
  end
  local dur = phase == "out" and OUT_MS or IN_MS
  local k = (t - widthAt) / dur
  width = widthFrom + (widthTo - widthFrom) * ((phase == "out") and easeOut(k) or backOut(k))
  if k >= 1 then
    width = widthTo
  end
  render(t)

  local moving = phase ~= "rest" or k < 1
  local mood = shown and shown.mood or "idle"
  if moving or (t >= blinkAt - 20 and t < blinkAt + 160) then
    setInterval(FAST)
  elseif mood == "work" then
    setInterval(FLICKER)
  elseif mood == "wait" then
    setInterval(BREATH)
  else
    setInterval(math.max(FAST, math.min(SLOW, blinkAt - t)))
  end
end

function onClick()
  act("panel", "open")
end

function onRightClick()
  if shown and (shown.kind == "done" or shown.kind == "edgecard") then
    act(shown.kind == "done" and "flash" or (shown.target or ""), "dismiss")
  end
end

if view then
  onView(view)
end
setInterval(FAST)
