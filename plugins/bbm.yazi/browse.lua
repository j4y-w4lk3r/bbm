--- @since 26.5.6

local lib = require(".lib")

local LOADING = { { name = "(loading…)", kind = "empty", key = "" } }
local BUCKETS_LOADING = { { name = "(loading buckets…)", kind = "empty", key = "" } }

local M = {
	keys = {
		{ on = "q", run = "quit" },
		{ on = "<Esc>", run = "quit" },
		{ on = "<Enter>", run = "enter" },
		{ on = "k", run = "up" },
		{ on = "j", run = "down" },
		{ on = "l", run = "enter" },
		{ on = "h", run = "back" },
		{ on = "B", run = "buckets" },
		{ on = "<Up>", run = "up" },
		{ on = "<Down>", run = "down" },
		{ on = "<Right>", run = "enter" },
		{ on = "<Left>", run = "back" },
		{ on = "r", run = "refresh" },
		{ on = "p", run = "pull" },
		{ on = "P", run = "pull_decrypt" },
	},
	permit = table.pack(ya.chan("mpsc", 1)),
}

function M:new(area)
	self:layout(area)
	return self
end

function M:layout(area)
	local chunks = ui.Layout()
		:constraints({
			ui.Constraint.Percentage(10),
			ui.Constraint.Percentage(80),
			ui.Constraint.Percentage(10),
		})
		:split(area)

	chunks = ui.Layout()
		:direction(ui.Layout.HORIZONTAL)
		:constraints({
			ui.Constraint.Percentage(5),
			ui.Constraint.Percentage(90),
			ui.Constraint.Percentage(5),
		})
		:split(chunks[2])

	self._area = chunks[2]
end

function M.fetch_buckets(self)
	lib.log("fetch_buckets: start")
	local started = os.clock()
	local output, err = lib.bbm_cmd(self, { "bucket", "list" }, { no_bucket = true })
		:stdout(Command.PIPED)
		:stderr(Command.PIPED)
		:output()
	local elapsed = os.clock() - started

	if not output then
		lib.log("fetch_buckets: no output in " .. string.format("%.2fs: %s", elapsed, tostring(err)))
		return nil, tostring(err)
	end

	lib.log(string.format(
		"fetch_buckets: done in %.2fs exit=%s",
		elapsed,
		tostring(output.status.code)
	))

	if not output.status.success then
		local msg = output.stderr or ("exit " .. tostring(output.status.code))
		return nil, msg
	end

	return M.parse_buckets(output.stdout or ""), nil
end

function M.fetch_ls(self, prefix)
	local args = { "ls", "--limit", "5000" }
	if prefix ~= "" then
		args[#args + 1] = prefix
	end

	lib.log("fetch_ls: bucket=" .. tostring(self.bucket) .. " prefix=" .. tostring(prefix))
	local started = os.clock()

	local output, err = lib.bbm_cmd(self, args):stdout(Command.PIPED):stderr(Command.PIPED):output()
	local elapsed = os.clock() - started

	if not output then
		lib.log("fetch_ls: no output in " .. string.format("%.2fs: %s", elapsed, tostring(err)))
		return nil, tostring(err)
	end

	lib.log(string.format(
		"fetch_ls: done in %.2fs exit=%s stderr=%s",
		elapsed,
		tostring(output.status.code),
		tostring(output.stderr or ""):gsub("\n", " ")
	))

	if not output.status.success then
		local msg = output.stderr or ("exit " .. tostring(output.status.code))
		return nil, msg
	end

	return M.parse_ls(prefix, output.stdout or ""), nil
end

function M:reflow()
	return { self }
end

function M:redraw()
	if not self._area then
		lib.log("redraw: no _area")
		return {}
	end

	local rows = {}
	local header = { "Name", "Size", "Type", "Modified" }
	local title

	if self.mode == "buckets" then
		title = "B2 buckets  (B=refresh list, j/k/l=navigate)"
		header = { "Bucket", "", "Created", "" }
		for _, e in ipairs(self.entries or {}) do
			if e.kind == "bucket" then
				rows[#rows + 1] = ui.Row({ e.name, "", e.ts or "", "bucket" })
			elseif e.kind == "error" or e.kind == "empty" then
				rows[#rows + 1] = ui.Row({ e.name, "", "", "" })
			end
		end
	else
		local bucket = self.bucket or "?"
		local path = self.prefix == "" and "/" or self.prefix
		title = "B2: " .. bucket .. ":" .. path .. "  (B=buckets, h=back)"
		for _, e in ipairs(self.entries or {}) do
			if e.kind == "dir" then
				rows[#rows + 1] = ui.Row({ e.name, "", "dir", "" })
			elseif e.kind == "error" or e.kind == "empty" then
				rows[#rows + 1] = ui.Row({ e.name, "", "", "" })
			else
				rows[#rows + 1] = ui.Row({ e.name, e.size or "", "file", e.ts or "" })
			end
		end
	end

	return {
		ui.Clear(self._area),
		ui.Border(ui.Edge.ALL)
			:area(self._area)
			:type(ui.Border.ROUNDED)
			:style(ui.Style():fg("blue"))
			:title(ui.Line(title):align(ui.Align.CENTER)),
		ui.Table(rows)
			:area(self._area:pad(ui.Pad(1, 2, 1, 2)))
			:header(ui.Row(header):style(ui.Style():bold()))
			:row(self.cursor or 0)
			:row_style(ui.Style():fg("blue"):underline())
			:widths({
				ui.Constraint.Percentage(50),
				ui.Constraint.Length(10),
				ui.Constraint.Length(6),
				ui.Constraint.Percentage(30),
			}),
	}
end

function M.parse_buckets(stdout)
	local entries = {}
	for line in stdout:gmatch("[^\r\n]+") do
		local ts, name = line:match("^(%d%d%d%d%-%d%d%-%d%d %d%d:%d%d:%d%d)%s+(%S+)$")
		if name then
			entries[#entries + 1] = { name = name, key = name, ts = ts, kind = "bucket" }
		end
	end
	table.sort(entries, function(a, b)
		return a.name < b.name
	end)
	if #entries == 0 then
		entries[1] = { name = "(no buckets)", kind = "empty", key = "" }
	end
	return entries
end

function M.parse_ls(prefix, stdout)
	local dirs, files = {}, {}

	for line in stdout:gmatch("[^\r\n]+") do
		local ts, size, key = line:match("^(%d%d%d%d%-%d%d%-%d%d %d%d:%d%d:%d%d)%s+(%S+)%s+(.+)$")
		if key and key ~= prefix then
			local rest = key:sub(#prefix + 1)
			local slash = rest:find("/")
			if slash then
				local dir = rest:sub(1, slash)
				dirs[dir] = true
			elseif rest ~= "" then
				files[#files + 1] = {
					name = rest,
					key = key,
					size = size,
					ts = ts,
					kind = "file",
				}
			end
		end
	end

	local entries = {}
	for dir in pairs(dirs) do
		entries[#entries + 1] = { name = dir, key = prefix .. dir, kind = "dir" }
	end
	table.sort(entries, function(a, b)
		return a.name < b.name
	end)

	table.sort(files, function(a, b)
		return a.name < b.name
	end)
	for _, f in ipairs(files) do
		entries[#entries + 1] = f
	end

	if #entries == 0 then
		entries[1] = { name = "(empty)", kind = "empty", key = "" }
	end
	return entries
end

function M:click() end

function M:scroll() end

function M:touch() end

return { M = M, LOADING = LOADING, BUCKETS_LOADING = BUCKETS_LOADING }
