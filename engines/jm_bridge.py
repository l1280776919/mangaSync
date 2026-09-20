#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
禁漫 (18comic / JMComic) JSON 命令行桥接脚本 —— 供 Go 后端调用。

依赖: jmcomic (hect0x7/JMComic-Crawler-Python, 只读复用 python3 环境（装了 jmcomic）/venv) + 标准库。
本脚本不修改 /srv/@appdata 下任何文件（仅只读读取 config.json 作为 cookie 兜底）。

协议（除 download 外，stdout 只输出一行 JSON）:
  ping
  login <username> <password>
  favorites                      # cookies 从环境变量 JM_COOKIES (JSON) 读
  search <keyword> <page> <sort> # sort: mr/mv/mp/tf
  detail <comicId>
  addfavorite <comicId>
  delfavorite <comicId>
  cover <comicId> <outPath>
  download <comicId> <baseDir> [--chapters 1,2]   # 逐行流式 JSON 事件

环境变量:
  JM_COOKIES        JSON 字符串，例如 {"AVS":"...","ipm5":"..."}
  JM_PROXY          代理，可空
  JM_IMAGE_WORKERS  图片并发，默认 8
  JM_PHOTO_WORKERS  章节并发，默认 2
  JM_CHAPTERS       可选，逗号分隔的章节 order（等同 --chapters）
  JM_FOLDER_ID      收藏夹 ID，默认 0
  JM_CONFIG         可选，config.json 路径（cookie 兜底），默认 
  JM_DEBUG=1        输出 jmcomic INFO 日志到 stderr（默认 WARNING）

所有日志一律走 stderr，stdout 只给 JSON。
"""
import argparse
import json
import logging
import os
import sys
import threading
import time
from pathlib import Path

# ----------------------------------------------------------------------------
# 常量与环境
# ----------------------------------------------------------------------------
JM_VENV_LIB = ""
DEFAULT_CONFIG_PATH = Path(os.environ.get("JM_CONFIG") or "")

# 固定优先使用的域名（域名列表本身仍保留其它域名做失败重试）
PREFER_API_DOMAIN = "www.cdnhjk.net"
PREFER_IMAGE_DOMAIN = "cdn-msp.jmapinodeudzn.net"

IMG_EXT = (".jpg", ".jpeg", ".png", ".gif", ".webp")
DEFAULT_IMAGE_WORKERS = 8
DEFAULT_PHOTO_WORKERS = 2
SORT_WHITELIST = ("mr", "mv", "mp", "tf")


def log(msg):
    """日志 -> stderr"""
    try:
        sys.stderr.write(f"[jm_bridge] {msg}\n")
        sys.stderr.flush()
    except Exception:
        pass


def emit(obj):
    """一行 JSON -> stdout（加锁保证并发下不被拆行）"""
    line = json.dumps(obj, ensure_ascii=False)
    with _STDOUT_LOCK:
        sys.stdout.write(line + "\n")
        sys.stdout.flush()


_STDOUT_LOCK = threading.Lock()


# ----------------------------------------------------------------------------
# jmcomic 导入（优先当前解释器环境，其次回落到 NAS 上的 venv）
# ----------------------------------------------------------------------------
def ensure_jmcomic():
    try:
        import jmcomic  # noqa: F401
        return
    except ImportError:
        pass
    import glob
    for sp in sorted(glob.glob(os.path.join(JM_VENV_LIB, "python*", "site-packages"))):
        if os.path.isdir(sp) and sp not in sys.path:
            sys.path.insert(0, sp)
    import jmcomic  # noqa: F401


def setup_logging():
    """把 jmcomic 的日志全部改到 stderr，避免污染 stdout 的 JSON 行。"""
    level = logging.INFO if os.environ.get("JM_DEBUG") == "1" else logging.WARNING
    lg = logging.getLogger("jmcomic")
    for h in list(lg.handlers):
        try:
            lg.removeHandler(h)
        except Exception:
            pass
    h = logging.StreamHandler(sys.stderr)
    h.setFormatter(logging.Formatter("[jmcomic] %(levelname)s %(message)s"))
    lg.addHandler(h)
    lg.setLevel(level)
    lg.propagate = False
    try:
        from jmcomic import JmModuleConfig
        JmModuleConfig.FLAG_ENABLE_JM_LOG = True
    except Exception:
        pass


# ----------------------------------------------------------------------------
# 配置 / cookies
# ----------------------------------------------------------------------------
def read_config_cookies():
    """只读读取 NAS 上 config.json 的 cookies（兜底用，绝不打印密码）"""
    try:
        data = json.loads(DEFAULT_CONFIG_PATH.read_text("utf-8"))
    except Exception as e:
        log(f"读取 {DEFAULT_CONFIG_PATH} 失败: {e}")
        return {}
    ck = data.get("cookies") or {}
    return {k: v for k, v in ck.items() if isinstance(v, str)} if isinstance(ck, dict) else {}


def env_cookies():
    raw = (os.environ.get("JM_COOKIES") or "").strip()
    if raw:
        try:
            ck = json.loads(raw)
            if isinstance(ck, dict) and ck:
                return {str(k): str(v) for k, v in ck.items() if v is not None}
        except Exception as e:
            log(f"JM_COOKIES 不是合法 JSON: {e}，尝试使用 config.json 的 cookies")
    ck = read_config_cookies()
    if ck:
        log("JM_COOKIES 未设置，回退使用 config.json 中的 cookies（只读）")
    return ck


def env_int(name, default):
    try:
        v = int(str(os.environ.get(name) or "").strip() or default)
        return v if v > 0 else default
    except Exception:
        return default


def pin_domains(client=None):
    """固定首选 API/图片域名，保证 ping 输出稳定、图片/封面可复现"""
    from jmcomic import JmModuleConfig

    apis = list(JmModuleConfig.DOMAIN_API_LIST)
    if PREFER_API_DOMAIN in apis:
        apis.remove(PREFER_API_DOMAIN)
    apis.insert(0, PREFER_API_DOMAIN)
    JmModuleConfig.DOMAIN_API_LIST = apis
    # 同时写入"已更新域名"，跳过服务端域名下发，保持确定性
    JmModuleConfig.DOMAIN_API_UPDATED_LIST = apis

    imgs = list(JmModuleConfig.DOMAIN_IMAGE_LIST)
    if PREFER_IMAGE_DOMAIN in imgs:
        imgs.remove(PREFER_IMAGE_DOMAIN)
    imgs.insert(0, PREFER_IMAGE_DOMAIN)
    JmModuleConfig.DOMAIN_IMAGE_LIST = imgs

    if client is not None:
        client.set_domain_list(apis)
    return apis[0]


def make_client(cookies=None, base_dir=None, rule="Bd/{Aid} {Aname}"):
    """构造 (option, client)；cookies 为 None 时不注入登录态。"""
    ensure_jmcomic()
    setup_logging()
    from jmcomic import JmOption

    meta = {}
    proxy = (os.environ.get("JM_PROXY") or "").strip()
    if proxy:
        meta["proxies"] = proxy
    if cookies:
        meta["cookies"] = dict(cookies)

    option = JmOption.construct({
        "dir_rule": {"base_dir": base_dir or "/tmp", "rule": rule, "normalize_zh": None},
        "download": {
            "cache": True,                      # 已存在的图片跳过（增量）
            "image": {"decode": True, "suffix": None},   # 保留原格式，不缩放
            "threading": {
                "image": env_int("JM_IMAGE_WORKERS", DEFAULT_IMAGE_WORKERS),
                "photo": env_int("JM_PHOTO_WORKERS", DEFAULT_PHOTO_WORKERS),
            },
        },
        "client": {"impl": "api", "postman": {"meta_data": meta}, "retry_times": 5},
        "log": False,
    })
    client = option.new_jm_client()
    if cookies:
        client["cookies"] = dict(cookies)
    pin_domains(client)
    return option, client


def api_domain():
    """当前实际会使用的 API 域名（不带协议）"""
    from jmcomic import JmModuleConfig
    try:
        return pin_domains()
    except Exception:
        return JmModuleConfig.DOMAIN_API_LIST[0]


# ----------------------------------------------------------------------------
# 小工具
# ----------------------------------------------------------------------------
def dir_stats(path):
    """(图片数, 字节数)"""
    p = Path(path)
    n = b = 0
    if not p.exists():
        return 0, 0
    for f in p.rglob("*"):
        if f.is_file() and f.suffix.lower() in IMG_EXT:
            n += 1
            try:
                b += f.stat().st_size
            except OSError:
                pass
    return n, b


def cleanup_empty_dirs(root):
    """清理库产生的空目录（自底向上）"""
    root = Path(root)
    if not root.exists():
        return
    for d in sorted((x for x in root.rglob("*") if x.is_dir()), key=lambda x: len(x.parts), reverse=True):
        try:
            d.rmdir()
        except OSError:
            pass


def sniff_content_type(path):
    try:
        with open(path, "rb") as f:
            head = f.read(16)
    except OSError:
        return "application/octet-stream"
    if head[:3] == b"\xff\xd8\xff":
        return "image/jpeg"
    if head[:8] == b"\x89PNG\r\n\x1a\n":
        return "image/png"
    if head[:4] == b"GIF8":
        return "image/gif"
    if head[:4] == b"RIFF" and head[8:12] == b"WEBP":
        return "image/webp"
    if head[:2] == b"BM":
        return "image/bmp"
    return "application/octet-stream"


def content_pair(item):
    """搜索/收藏页的一个条目 -> (id:str, data:dict)"""
    if isinstance(item, (tuple, list)):
        cid = item[0] if len(item) > 0 else None
        data = item[1] if len(item) > 1 else {}
    else:
        cid = getattr(item, "id", None)
        data = getattr(item, "src_dict", None)

    if isinstance(data, dict):
        cid = cid if cid is not None else data.get("id")
        return str(cid or ""), data

    # 收藏页的条目是 (id, 标题字符串)
    if isinstance(data, str):
        return str(cid or ""), {"id": str(cid or ""), "name": data}

    # data 可能是 JmAlbumDetail 之类的对象
    from jmcomic import JmAlbumDetail, JmcomicText
    if isinstance(data, JmAlbumDetail):
        return str(data.album_id), {
            "id": data.album_id, "name": data.name, "author": ",".join(data.authors or []),
            "tags": list(data.tags or []),
        }
    if cid is None and data is not None:
        cid = getattr(data, "album_id", None) or getattr(data, "id", None)
    return str(cid or ""), {}


def cover_url(comic_id, size=""):
    from jmcomic import JmcomicText
    return JmcomicText.get_album_cover_url(str(comic_id), image_domain=PREFER_IMAGE_DOMAIN, size=size)


def chapter_image_count(client, album, photo, multi):
    """章节图片数：单章本直接用 album.page_count，多章本拉一次章节详情"""
    if not multi:
        try:
            return int(album.page_count or 0)
        except Exception:
            pass
    detail = client.get_photo_detail(photo.photo_id, False)
    return int(len(detail))


def parse_orders(text):
    """'1,2, 3' -> {1,2,3}；空 -> None"""
    if not text:
        return None
    out = set()
    for part in str(text).replace("，", ",").split(","):
        part = part.strip()
        if part:
            try:
                out.add(int(part))
            except ValueError:
                log(f"忽略非法章节号: {part!r}")
    return out or None


# ----------------------------------------------------------------------------
# 子命令
# ----------------------------------------------------------------------------
def cmd_ping(args):
    emit({"ok": True, "impl": "jm", "domain": "https://" + api_domain()})
    return 0


def cmd_login(args):
    ensure_jmcomic()
    op, client = make_client(cookies={})
    resp = client.login(args.username, args.password)
    data = resp.res_data
    cookies = dict(client["cookies"] or {})
    emit({
        "cookies": cookies,
        "username": str(data.get("username") or args.username),
        "nickname": str(data.get("nickname") or data.get("fname") or data.get("username") or args.username),
        "uid": str(data.get("uid") or ""),
        "favoritesCount": int(data.get("album_favorites") or 0),
        "favoritesMax": int(data.get("album_favorites_max") or 0),
    })
    return 0


def _favorite_items(client, folder_id="0", order="mr"):
    items, seen, pages = [], set(), 0
    for page in client.favorite_folder_gen(folder_id=folder_id):
        pages += 1
        new = 0
        for it in page:
            cid, data = content_pair(it)
            if not cid or cid in seen:
                continue
            seen.add(cid)
            new += 1
            items.append({
                "comicId": cid,
                "title": str(data.get("name") or ""),
            })
        if new == 0 or pages > 200:
            break
    return items


def cmd_favorites(args):
    cookies = env_cookies()
    if not cookies:
        raise RuntimeError("没有可用登录态（JM_COOKIES 为空且 config.json 无 cookies）")
    op, client = make_client(cookies=cookies)
    items = _favorite_items(client, folder_id=(os.environ.get("JM_FOLDER_ID") or "0").strip() or "0")
    emit({"items": items})
    return 0


def cmd_search(args):
    sort = (args.sort or "mr").strip()
    if sort not in SORT_WHITELIST:
        sort = "mr"
    op, client = make_client(cookies=env_cookies())
    page = client.search(args.keyword, max(1, int(args.page or 1)), 0, sort, "all", "", None)

    items = []
    for it in page:
        cid, data = content_pair(it)
        if not cid:
            continue
        cat = data.get("category") or {}
        cat_title = cat.get("title") if isinstance(cat, dict) else ""
        author = data.get("author")
        if isinstance(author, (list, tuple)):
            author = ",".join(str(a) for a in author)
        tags = data.get("tags") or []
        if isinstance(tags, str):
            tags = [t for t in tags.replace(",", " ").split() if t]
        items.append({
            "comicId": cid,
            "title": str(data.get("name") or data.get("title") or ""),
            "author": str(author or ""),
            "category": str(cat_title or ""),
            "tags": [str(t) for t in tags],
            "cover": cover_url(cid),
        })
    emit({"total": int(page.total or 0), "items": items})
    return 0


def cmd_detail(args):
    op, client = make_client(cookies=env_cookies())
    album = client.get_album_detail(args.comicId)
    multi = len(album) > 1

    chapters = []
    for photo in album:
        try:
            images = chapter_image_count(client, album, photo, multi)
        except Exception as e:
            log(f"章节 {photo.index} 详情获取失败: {e}")
            images = 0
        chapters.append({
            "order": int(photo.index),
            "id": str(photo.photo_id),
            "title": str(photo.name or ""),
            "images": images,
        })
    emit({
        "comicId": str(album.album_id),
        "title": str(album.name or ""),
        "author": ",".join(str(a) for a in (album.authors or [])),
        "category": "",
        "tags": [str(t) for t in (album.tags or [])],
        "description": str(album.description or ""),
        "cover": cover_url(album.album_id),
        "chapters": chapters,
    })
    return 0


def _set_favorite(client, comic_id, want: bool):
    """幂等设置收藏状态: want=True 加收藏, False 取消收藏"""
    album = client.get_album_detail(comic_id)   # 含 is_favorite 字段
    cur = bool(getattr(album, "is_favorite", False))
    if cur == want:
        log(f"收藏状态已是 {want}，跳过（幂等）")
        return True
    resp = client.toggle_favorite_album(comic_id)
    actual = str(resp.model_data.type or "")
    expected = "add" if want else "remove"
    if actual != expected:
        # 说明状态被外部改动（并发），立刻改回期望状态
        log(f"收藏操作返回 {actual!r}，与期望 {expected!r} 不符，尝试回滚")
        try:
            client.toggle_favorite_album(comic_id)
        except Exception as e:
            log(f"回滚失败: {e}")
        if actual == "remove" and want:
            raise RuntimeError("目标已处于收藏状态")
        if actual == "add" and not want:
            raise RuntimeError("目标未处于收藏状态")
    return True


def cmd_addfavorite(args):
    op, client = make_client(cookies=env_cookies())
    _set_favorite(client, args.comicId, True)
    emit({"ok": True})
    return 0


def cmd_delfavorite(args):
    op, client = make_client(cookies=env_cookies())
    _set_favorite(client, args.comicId, False)
    emit({"ok": True})
    return 0


def cmd_cover(args):
    import urllib.request

    out_path = Path(args.outPath).expanduser()
    out_path.parent.mkdir(parents=True, exist_ok=True)
    op, client = make_client(cookies=env_cookies())
    url = cover_url(args.comicId)
    ok = False
    try:
        client.download_album_cover(args.comicId, str(out_path), size="")
        ok = out_path.exists() and out_path.stat().st_size > 0
    except Exception as e:
        log(f"库内封面下载失败({e})，回退直连下载")
    if not ok:
        req = urllib.request.Request(url, headers={
            "User-Agent": "Mozilla/5.0 (Linux; Android 10) AppleWebKit/537.36 Chrome/120 Mobile Safari/537.36",
            "Referer": "https://" + api_domain() + "/",
        })
        with urllib.request.urlopen(req, timeout=60) as r, open(out_path, "wb") as f:
            f.write(r.read())
    nbytes = out_path.stat().st_size
    emit({"path": str(out_path), "bytes": nbytes, "contentType": sniff_content_type(out_path)})
    return 0


# ---------------- download ----------------
class ProgressHub:
    """把下载过程转成逐行 JSON 事件（线程安全）"""

    def __init__(self):
        self.lock = threading.Lock()
        self.cur = None           # 当前章节状态
        self.totals = [0, 0]      # [图片数, 字节数]（以磁盘统计为准）

    # --- 章节 ---
    def chapter_start(self, photo):
        total = len(photo)
        with self.lock:
            self.cur = {"order": int(photo.index), "title": str(photo.name or ""),
                        "total": total, "done": 0, "bytes": 0,
                        "t0": time.time(), "last": 0.0}
        emit({"event": "chapter", "order": int(photo.index), "title": str(photo.name or ""),
              "images": total, "state": "start"})

    def image_done(self, image, img_save_path):
        snap = None
        try:
            size = os.path.getsize(img_save_path)
        except OSError:
            size = 0
        with self.lock:
            c = self.cur
            if c is None:
                return
            c["done"] += 1
            c["bytes"] += size
            now = time.time()
            # 每秒最多推送一次，最后一张必推
            if c["done"] >= c["total"] or (now - c["last"] >= 1.0):
                c["last"] = now
                elapsed = max(now - c["t0"], 1e-3)
                snap = {
                    "event": "progress", "order": c["order"],
                    "imagesDone": c["done"], "imagesTotal": c["total"],
                    "bytes": c["bytes"], "speedBps": int(c["bytes"] / elapsed),
                }
        if snap:
            emit(snap)

    def chapter_finish(self, photo, path, images, nbytes):
        with self.lock:
            self.cur = None
            self.totals[0] += images
            self.totals[1] += nbytes
        emit({"event": "chapter", "order": int(photo.index), "title": str(photo.name or ""),
              "state": "done", "images": images, "bytes": nbytes, "path": str(path)})


def build_downloader_class(hub):
    ensure_jmcomic()
    from jmcomic import JmDownloader

    class BridgeDownloader(JmDownloader):
        def __init__(self, option):
            super().__init__(option)
            self.hub = hub

        def before_photo(self, photo):
            super().before_photo(photo)
            self.hub.chapter_start(photo)

        def after_image(self, image, img_save_path):
            super().after_image(image, img_save_path)
            self.hub.image_done(image, img_save_path)

    return BridgeDownloader


def cmd_download(args):
    from jmcomic import DirRule

    comic_id = str(args.comicId)
    base_dir = Path(args.baseDir).expanduser()
    base_dir.mkdir(parents=True, exist_ok=True)

    only = parse_orders(args.chapters) or parse_orders(os.environ.get("JM_CHAPTERS"))

    op, client = make_client(cookies=env_cookies(), base_dir=str(base_dir))
    album = client.get_album_detail(comic_id)
    multi = len(album) > 1
    rule = "Bd/{Aid} {Aname}" if not multi else "Bd/{Aid} {Aname}/{Pindex} {Pname}"
    op.dir_rule = DirRule(rule=rule, base_dir=str(base_dir), normalize_zh=None)

    photos = [p for p in album if only is None or int(p.index) in only]
    if not photos:
        emit({"event": "error", "msg": f"没有可下载的章节（chapters={sorted(only) if only else 'all'}）"})
        return 1

    album_root = Path(op.dir_rule.decide_album_root_dir(album))

    # 预取章节详情（拿到每章图片数与图片地址；库内有缓存，后续下载不会重复请求）
    broken = {}
    for photo in photos:
        try:
            client.check_photo(photo)
        except Exception as e:
            broken[int(photo.index)] = str(e)[:200]
            log(f"章节 {photo.index} 详情失败: {e}")

    images_total = 0
    for photo in photos:
        if int(photo.index) in broken:
            continue
        images_total += len(photo)

    emit({"event": "start", "comicId": str(album.album_id), "title": str(album.name or ""),
          "chaptersTotal": len(photos), "imagesTotal": images_total})

    hub = ProgressHub()
    downloader = build_downloader_class(hub)(op)
    downloader.client = client   # 复用已登录 client

    hard_fail = False
    for photo in photos:
        order = int(photo.index)
        if order in broken:
            hard_fail = True
            emit({"event": "error", "order": order, "msg": f"章节详情获取失败: {broken[order]}"})
            continue

        chapter_dir = Path(op.decide_image_save_dir(photo))
        expected = len(photo)
        failed_before = len(downloader.download_failed_image)
        try:
            downloader.download_by_photo_detail(photo)
        except Exception as e:
            log(f"章节 {order} 下载异常: {e}")
            try:
                failed_before = failed_before  # noqa
            except Exception:
                pass
            emit({"event": "error", "order": order, "msg": str(e)[:300]})

        images, nbytes = dir_stats(chapter_dir)
        if images < expected:
            hard_fail = True
            new_fails = downloader.download_failed_image[failed_before:]
            if new_fails:
                img, err = new_fails[0]
                emit({"event": "error", "order": order,
                      "msg": f"{len(new_fails)} 张图片下载失败，第一张: {str(err)[:180]}"})
            else:
                emit({"event": "error", "order": order,
                      "msg": f"图片数不足: {images}/{expected}"})

        hub.chapter_finish(photo, chapter_dir, images, nbytes)

    # 清理库产生的空目录
    cleanup_empty_dirs(album_root)

    if multi:
        done_path = album_root
    else:
        done_path = Path(op.decide_image_save_dir(photos[0]))
    images, nbytes = dir_stats(done_path)

    emit({"event": "done", "comicId": str(album.album_id), "path": str(done_path),
          "images": images, "bytes": nbytes})
    return 1 if hard_fail else 0


# ----------------------------------------------------------------------------
# main
# ----------------------------------------------------------------------------
def main(argv=None):
    ap = argparse.ArgumentParser(prog="jm_bridge.py", description="禁漫 JSON 桥接")
    sub = ap.add_subparsers(dest="cmd")

    sub.add_parser("ping")

    p = sub.add_parser("login")
    p.add_argument("username")
    p.add_argument("password")

    sub.add_parser("favorites")

    p = sub.add_parser("search")
    p.add_argument("keyword")
    p.add_argument("page", nargs="?", default=1)
    p.add_argument("sort", nargs="?", default="mr")

    p = sub.add_parser("detail")
    p.add_argument("comicId")

    p = sub.add_parser("addfavorite")
    p.add_argument("comicId")

    p = sub.add_parser("delfavorite")
    p.add_argument("comicId")

    p = sub.add_parser("cover")
    p.add_argument("comicId")
    p.add_argument("outPath")

    p = sub.add_parser("download")
    p.add_argument("comicId")
    p.add_argument("baseDir")
    p.add_argument("--chapters", default=None)

    args = ap.parse_args(argv)

    handlers = {
        "ping": cmd_ping,
        "login": cmd_login,
        "favorites": cmd_favorites,
        "search": cmd_search,
        "detail": cmd_detail,
        "addfavorite": cmd_addfavorite,
        "delfavorite": cmd_delfavorite,
        "cover": cmd_cover,
        "download": cmd_download,
    }
    if args.cmd not in handlers:
        ap.print_usage(sys.stderr)
        return 2

    try:
        return handlers[args.cmd](args) or 0
    except Exception as e:
        log(f"{args.cmd} 失败: {e}")
        if args.cmd == "download":
            emit({"event": "error", "msg": str(e)[:300]})
        else:
            emit({"ok": False, "error": str(e)[:300]})
        return 1


if __name__ == "__main__":
    sys.exit(main())
