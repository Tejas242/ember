--!nonstrict
-- Ember, drawn as SVG from the live palette. Frames are written once per
-- palette into the plugin data dir; the widget stacks a mood's frames and
-- switches them by opacity, so animating never re-decodes an image.

local M = {}

M.W, M.H = 40, 48 -- viewBox

local function hex(c)
  c = (c or "#888888"):gsub("#", "")
  return tonumber(c:sub(1, 2), 16) or 136, tonumber(c:sub(3, 4), 16) or 136, tonumber(c:sub(5, 6), 16) or 136
end

function M.mix(a, b, t)
  local r1, g1, b1 = hex(a)
  local r2, g2, b2 = hex(b)
  local f = function(x, y) return math.floor(x + (y - x) * t + 0.5) end
  return string.format("#%02x%02x%02x", f(r1, r2), f(g1, g2), f(b1, b2))
end

local function flamePath(sway, stretch)
  local r, x, y = 13, 20, 33
  local k = r * 0.5523
  local tx, ty = x + r * 0.3 + sway, y - 30 * stretch
  local lx, ly = x - r * 0.9 + sway * 0.4, y - r * 1.45 * stretch
  local f = function(n) return string.format("%.2f", n) end
  return table.concat({
    "M", f(tx), f(ty),
    "C", f(tx + r * 0.12), f(ty + r * 0.8), f(x + r * 1.08), f(y - r * 0.95), f(x + r), f(y),
    "C", f(x + r), f(y + k), f(x + k), f(y + r), f(x), f(y + r),
    "C", f(x - k), f(y + r), f(x - r), f(y + k), f(x - r), f(y),
    "C", f(x - r * 1.06), f(y - r * 0.55), f(x - r * 1.12), f(y - r * 1.1), f(lx), f(ly),
    "C", f(x - r * 0.72), f(y - r * 1.2), f(x - r * 0.48), f(y - r * 1.08), f(x - r * 0.28), f(y - r * 1.12),
    "C", f(x - r * 0.08), f(y - r * 1.5), f(tx - r * 0.32), f(ty + r * 0.62), f(tx), f(ty), "Z",
  }, " ")
end

-- eyes: "open" | "blink" | "happy" | "up" | "side"
local function svg(body, ink, sway, stretch, eyes)
  local core = M.mix(body, "#ffffff", 0.6)
  local eh, ey = 6.2, 34.5
  if eyes == "blink" then
    eh = 1.2
  elseif eyes == "happy" then
    eh, ey = 1.7, 33.5
  elseif eyes == "up" then
    ey = 32.5
  elseif eyes == "side" then
    eh = 3.6
  end
  local ex = sway * 0.3
  local eye = function(cx)
    return string.format('<rect x="%.2f" y="%.2f" width="3.4" height="%.2f" rx="1.7" fill="%s"/>', cx - 1.7 + ex, ey - eh / 2, eh, ink)
  end
  return table.concat({
    string.format('<svg xmlns="http://www.w3.org/2000/svg" width="80" height="96" viewBox="0 0 %d %d">', M.W, M.H),
    string.format('<path d="%s" fill="%s"/>', flamePath(sway, stretch), body),
    string.format('<g transform="translate(20 41) scale(0.5) translate(-20 -33)"><path d="%s" fill="%s"/></g>', flamePath(sway * 0.6, stretch), core),
    eye(15.5), eye(24.5),
    "</svg>",
  })
end

-- moods → palette role and the frames that animate it
M.moods = {
  idle  = { role = "on_surface_variant", frames = { { 0, 1.00, "open" }, { 0, 1.00, "blink" } } },
  work  = { role = "primary", frames = {
    { 0.0, 1.00, "open" }, { 1.6, 1.04, "open" }, { 2.4, 0.98, "open" },
    { 0.8, 1.05, "open" }, { -1.4, 0.99, "open" }, { -2.2, 1.03, "open" }, { 0.4, 1.00, "blink" },
  } },
  wait  = { role = "tertiary", frames = { { 0, 1.00, "up" }, { 0, 1.06, "up" }, { 0, 1.00, "blink" } } },
  happy = { role = "primary", frames = { { 0, 1.00, "happy" } } },
  error = { role = "error", frames = { { 0, 1.00, "open" }, { 0, 1.00, "blink" } } },
  nudge = { role = "secondary", frames = { { 0, 1.00, "side" }, { 0, 1.00, "blink" } } },
}

-- render writes every frame for the current palette and returns the folder
-- plus a key that changes when the palette does.
function M.render(dataDir)
  local roles, key = {}, {}
  for _, role in ipairs({ "primary", "secondary", "tertiary", "error", "on_surface_variant", "surface" }) do
    roles[role] = noctalia.getColor(role) or "#888888"
    table.insert(key, roles[role])
  end
  local stamp = table.concat(key, ""):gsub("#", "")
  local dir = dataDir .. "/ember-" .. stamp
  if noctalia.fileExists(dir .. "/done") then
    return dir, stamp
  end
  noctalia.mkdirAll(dir)
  local ink = M.mix(roles.surface, "#000000", 0.35)
  for name, m in pairs(M.moods) do
    for i, fr in ipairs(m.frames) do
      noctalia.writeFile(string.format("%s/%s-%d.svg", dir, name, i), svg(roles[m.role], ink, fr[1], fr[2], fr[3]))
    end
  end
  noctalia.writeFile(dir .. "/done", "1")
  -- drop frames of older palettes
  for _, f in ipairs(noctalia.listDir(dataDir) or {}) do
    if f:sub(1, 6) == "ember-" and f ~= "ember-" .. stamp then
      for _, g in ipairs(noctalia.listDir(dataDir .. "/" .. f) or {}) do
        noctalia.removeFile(dataDir .. "/" .. f .. "/" .. g)
      end
    end
  end
  return dir, stamp
end

return M
