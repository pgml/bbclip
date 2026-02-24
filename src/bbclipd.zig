//! This file is the daemon for bbclip which is going to be the long-running
//! background part of bbclip.
//! It monitors all clipboard interactions and all io interactions with
//! the clipboard history file.
//!
//! Still WIP and not in use as of yet.
const std = @import("std");
const known_folders = @import("known-folders");

pub const KnownFolderConfig = struct {
    xdg_force_default: bool = false,
    xdg_on_mac: bool = false,
};

const socket_path = "/tmp/bbclip.sock";

const Clipboard = struct {
    pub const Entry = struct {
        id: u16,
        str: []const u8,
    };

    alloc: std.mem.Allocator,

    filename: []const u8 = "org.pgml.bbclip-hist",

    max_entries: u16 = 100,

    path: []const u8 = "",

    content: std.ArrayList(Entry) = .empty,

    mutex: std.Thread.Mutex = .{},

    cond: std.Thread.Condition = .{},

    pub fn init(alloc: std.mem.Allocator) !Clipboard {
        var self: Clipboard = .{ .alloc = alloc };

        if (try known_folders.getPath(alloc, .data)) |dir| {
            self.path = try std.fmt.allocPrint(alloc, "{s}/{s}", .{
                dir,
                self.filename,
            });
        }

        try self.readFile();

        return self;
    }

    fn readFile(self: *Clipboard) !void {
        const file = try std.fs.openFileAbsolute(self.path, .{ .mode = .read_only });
        defer file.close();

        const stat = try file.stat();

        if (stat.size == 0) {
            return;
        }

        const buf = try self.alloc.alloc(u8, stat.size);
        defer self.alloc.free(buf);

        var reader = file.reader(buf);
        try reader.interface.readSliceAll(buf);

        const parsed = try std.json.parseFromSlice([][]const u8, self.alloc, buf, .{});
        defer parsed.deinit();
        errdefer parsed.deinit();

        var i: u16 = 0;
        for (parsed.value) |val| {
            try self.content.append(
                self.alloc,
                .{
                    .id = i,
                    .str = try self.alloc.dupe(u8, val),
                },
            );
            i += 1;
        }
    }

    pub fn write(self: *Clipboard) !void {
        self.mutex.lock();
        defer self.mutex.unlock();

        var content: std.ArrayList([]const u8) = .empty;
        defer {
            for (content.items) |item| {
                self.alloc.free(item);
            }
            content.deinit(self.alloc);
        }

        for (self.content.items) |item| {
            try content.append(self.alloc, item.str);
        }

        var out = std.Io.Writer.Allocating.init(self.alloc);
        defer out.deinit();

        var stringifier = std.json.Stringify{ .writer = &out.writer };
        try stringifier.write(try content.toOwnedSlice(self.alloc));

        const file = try std.fs.openFileAbsolute(self.path, .{
            .mode = .read_write,
            .lock = .exclusive,
        });
        defer file.close();

        //std.log.debug("{s}", .{out.writer.buffered()});

        const stat = try file.stat();
        const buf = try self.alloc.alloc(u8, stat.size);
        var writer = file.writer(buf);

        const n = try writer.interface.write(out.writer.buffered());

        writer.interface.flush() catch |err| {
            std.log.info("Failed to update clipboard: {}", .{err});
            return;
        };

        std.log.info("clipboard updated. {} bytes written. Clipboard len: {}", .{
            n,
            self.len(),
        });
    }

    pub fn jsonStringify(self: Clipboard) ![]const u8 {
        var out = std.Io.Writer.Allocating.init(self.alloc);
        defer out.deinit();

        var stringifier = std.json.Stringify{ .writer = &out.writer };
        try stringifier.beginArray();
        for (self.content.items) |item| {
            try stringifier.beginObject();
            {
                try stringifier.objectField("id");
                try stringifier.write(item.id);
                try stringifier.objectField("content");
                try stringifier.write(item.str);
            }
            try stringifier.endObject();
        }
        try stringifier.endArray();
        //try stringifier.write("\n");
        const out_str = out.writer.buffered();

        return try self.alloc.dupe(u8, out_str);
    }

    pub fn jsonLen(self: Clipboard) !usize {
        const json = try self.jsonStringify();
        defer self.alloc.free(json);

        //var len_buf: [16]u8 = undefined;
        //const len = try std.fmt.bufPrint(&len_buf, "{}", .{
        //    json.len,
        //});

        const length = json.len;
        return length;
    }

    pub fn len(self: Clipboard) usize {
        return self.content.items.len;
    }

    pub fn deinit(self: *Clipboard) void {
        for (self.content.items) |item| {
            self.alloc.free(item.str);
        }
        self.content.deinit(self.alloc);
        self.alloc.free(self.path);
    }
};

pub fn main() !u8 {
    try daemonize();

    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    const alloc = gpa.allocator();

    if (try connectSocket()) {
        std.log.info("Another instance already running. Exiting.", .{});
        return 1;
    }

    try listenSocket(alloc);

    return 0;
}

fn connectSocket() !bool {
    const sock = try createSocket();
    defer std.posix.close(sock.sock);

    std.posix.connect(sock.sock, @ptrCast(&sock.addr), sock.len) catch {
        return false;
    };

    //_ = try std.posix.write(sock.sock, "SHOW\n");

    return true;
}

fn listenSocket(alloc: std.mem.Allocator) !void {
    std.fs.cwd().deleteFile(socket_path) catch |err| switch (err) {
        error.FileNotFound => {},
        else => |e| {
            std.log.err("failed to remove old socket file '{s}': {s}", .{
                socket_path,
                @errorName(e),
            });
            return;
        },
    };

    var sock = try createSocket();
    defer std.posix.close(sock.sock);
    errdefer std.posix.close(sock.sock);

    try std.posix.bind(sock.sock, @ptrCast(&sock.addr), sock.len);
    std.log.info("bound socket to '{s}'", .{socket_path});

    try std.posix.listen(sock.sock, 8);
    std.log.info("listening...'", .{});

    var clipboard: Clipboard = Clipboard.init(alloc) catch |err| {
        std.log.err(
            "Failed to initialise clipboard. {s} exiting...",
            .{@errorName(err)},
        );
        std.process.exit(0);
        return;
    };
    defer clipboard.deinit();

    const thread: std.Thread = try std.Thread.spawn(
        .{ .allocator = alloc },
        worker,
        .{ &clipboard, &sock },
    );
    thread.detach();

    const out_buf = try alloc.alloc(u8, 100 * 1024);
    defer alloc.free(out_buf);

    while (true) {
        std.Thread.sleep(300 * std.time.ns_per_ms);

        const argv = [_][]const u8{ "wl-paste", "--no-newline" };
        var proc: std.process.Child = .init(&argv, alloc);

        proc.stdin_behavior = .Ignore;
        proc.stdout_behavior = .Pipe;
        proc.stderr_behavior = .Ignore;

        try proc.spawn();

        const stdout = proc.stdout orelse continue;
        var reader = stdout.reader(out_buf);
        const n = try reader.interface.readSliceShort(out_buf);
        const out = out_buf[0..n];

        _ = try proc.wait();

        if (std.mem.eql(u8, out, "")) {
            continue;
        }

        var update = false;
        var last: []const u8 = "";

        if (clipboard.len() >= clipboard.max_entries) {
            std.log.debug("{} {}", .{
                clipboard.max_entries,
                clipboard.content.items.len,
            });
            while (clipboard.len() >= clipboard.max_entries) {
                const entry = clipboard.content.orderedRemove(0);
                clipboard.alloc.free(entry.str);
            }
            //clipboard.content.shrinkAndFree(clipboard.alloc, clipboard.max_entries);
            //try clipboard.write();
        }

        if (clipboard.content.items.len > 0) {
            last = clipboard.content.getLast().str;
        }

        const img = false;
        if (img) {
            // @todo handle image
        } else {
            if (!std.mem.eql(u8, out, last)) {
                update = true;
            }
        }

        if (update) {
            try clipboard.content.append(alloc, .{
                .id = @intCast(clipboard.content.items.len),
                .str = try alloc.dupe(u8, out),
            });
            try clipboard.write();
        }
    }
}

fn worker(clipboard: *Clipboard, sock: *Socket) !void {
    while (true) {
        const conn = try std.posix.accept(
            sock.sock,
            @ptrCast(&sock.addr),
            &sock.len,
            0,
        );
        defer std.posix.close(conn);
        errdefer std.posix.close(conn);

        var buf: [128]u8 = undefined;
        const n = std.posix.read(conn, &buf) catch {
            std.posix.close(conn);
            continue;
        };

        if (n == 0) {
            std.posix.close(conn);
            continue;
        }

        if (std.mem.eql(u8, buf[0..n], "GET_HIST_LEN\n")) {
            //_ = std.posix.write(conn, try clipboard.jsonLen()) catch {};
        }

        if (std.mem.eql(u8, buf[0..n], "GET_HIST\n")) {
            const str = try clipboard.jsonStringify();
            defer clipboard.alloc.free(str);
            _ = std.posix.write(conn, str) catch {};
        }
    }
}

const Socket = struct {
    sock: std.posix.socket_t,
    addr: std.posix.sockaddr.un,
    len: u32,
};

fn createSocket() !Socket {
    const sock = try std.posix.socket(
        std.posix.AF.UNIX,
        std.posix.SOCK.STREAM,
        0,
    );

    var sockaddr = std.posix.sockaddr.un{
        .family = std.posix.AF.UNIX,
        .path = undefined,
    };

    @memset(&sockaddr.path, 0);
    if (socket_path.len >= sockaddr.path.len) {
        std.log.err("socket path too long", .{});
        return false;
    }
    @memcpy(sockaddr.path[0..socket_path.len], socket_path);

    const len = @sizeOf(std.posix.sa_family_t) + socket_path.len + 1;

    return .{
        .sock = sock,
        .addr = sockaddr,
        .len = len,
    };
}

pub fn daemonize() !void {
    const pid = try std.posix.fork();
    if (pid > 0) {
        std.process.exit(0);
    }

    _ = try std.posix.setsid();

    const pid2 = try std.posix.fork();
    if (pid2 > 0) {
        std.process.exit(0);
    }

    _ = std.c.umask(0);

    try std.posix.chdir("/");

    const devnull = try std.posix.open("/dev/null", .{ .ACCMODE = .RDWR }, 0);
    try std.posix.dup2(devnull, 0);
    try std.posix.dup2(devnull, 1);

    if (devnull > 1) {
        std.posix.close(devnull);
    }
}
