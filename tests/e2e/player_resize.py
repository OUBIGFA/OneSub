"""Windows first import and rapid resizing, measured from real media pixels.

Requires pywin32 and Pillow in the developer environment. The test owns its
process/data/IPC directory and uses the application import argument and
PrintWindow. It never enables debugging or changes an installed instance.
"""
import argparse
import ctypes
import json
import os
from pathlib import Path
import subprocess
import time

from PIL import Image, ImageChops
import win32con
import win32gui
import win32process
import win32ui


def capture(hwnd):
    left, top, right, bottom = win32gui.GetWindowRect(hwnd)
    w, h = right-left, bottom-top
    dc = win32gui.GetWindowDC(hwnd)
    src = win32ui.CreateDCFromHandle(dc)
    dst = src.CreateCompatibleDC()
    bmp = win32ui.CreateBitmap()
    bmp.CreateCompatibleBitmap(src, w, h)
    dst.SelectObject(bmp)
    try:
        if not ctypes.windll.user32.PrintWindow(hwnd, dst.GetSafeHdc(), 2):
            raise RuntimeError('PrintWindow failed')
        return Image.frombuffer('RGB', (w, h), bmp.GetBitmapBits(True), 'raw', 'BGRX', 0, 1).copy()
    finally:
        win32gui.DeleteObject(bmp.GetHandle())
        dst.DeleteDC(); src.DeleteDC(); win32gui.ReleaseDC(hwnd, dc)


def picture_aspect(image):
    # Detect the self-authored red picture independently of DOM/layout bounds.
    r, g, _ = image.split()
    mask = ImageChops.multiply(r.point(lambda x: 255 if x > 180 else 0), g.point(lambda x: 255 if x < 60 else 0))
    # Status-bar errors may also be red; only the upper media region counts.
    mask.paste(0, (0, int(image.height*.72), image.width, image.height))
    mask.paste(0, (int(image.width*.65), 0, image.width, image.height))
    box = mask.getbbox()
    return ((box[2]-box[0])/(box[3]-box[1]), box) if box else None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--exe', required=True)
    parser.add_argument('--work', required=True)
    parser.add_argument('--baseline', action='store_true')
    parser.add_argument('--no-subtitles', action='store_true')
    parser.add_argument('--frame-size', choices=('320x180', '180x320', '320x240'), default='320x180')
    parser.add_argument('--development', action='store_true')
    args = parser.parse_args()
    ctypes.windll.user32.SetProcessDPIAware()
    work = Path(args.work).resolve(); work.mkdir(parents=True, exist_ok=False)
    video = work/'resize.mp4'
    width, height = map(int, args.frame_size.split('x'))
    expected_aspect = width / height
    subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i',f'color=c=red:size={args.frame_size}:rate=30:duration=4','-c:v','libx264','-pix_fmt','yuv420p',str(video)], check=True)
    if not args.no_subtitles:
        (work/'resize.srt').write_text('1\n00:00:00,000 --> 00:00:04,000\nResize check\n', encoding='utf-8')
    (work/'data').mkdir(); (work/'ipc').mkdir()
    (work/'data/settings.json').write_text('{"version":5,"uiLanguage":"zh-Hans"}', encoding='utf-8')
    env = dict(os.environ, ONESUB_DATA_ROOT=str(work/'data'), XDG_RUNTIME_DIR=str(work/'ipc'), WEBVIEW2_USER_DATA_FOLDER=str(work/'webview'))
    # Verify the product's runtime configuration, not a developer's browser
    # flags (which could mask the first-frame defect or enable debugging).
    env.pop('WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS', None)
    if not args.development:
        for key in ('ONESUB_FFMPEG','ONESUB_FFPROBE','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT'):
            env.pop(key,None)
    command=[str(Path(args.exe).resolve()),str(video)]
    process = subprocess.Popen(command, env=env, stderr=(work/'stderr.log').open('w'))
    hwnd = None
    try:
        for _ in range(150):
            if process.poll() is not None: raise RuntimeError(f'test process exited: {process.returncode}')
            windows=[]
            def collect(h, _):
                if win32gui.IsWindowVisible(h) and win32process.GetWindowThreadProcessId(h)[1]==process.pid and not win32gui.GetWindow(h,win32con.GW_OWNER): windows.append(h)
            win32gui.EnumWindows(collect,None)
            if windows:
                hwnd=max(windows,key=lambda h:(lambda r:(r[2]-r[0])*(r[3]-r[1]))(win32gui.GetWindowRect(h)))
                break
            time.sleep(.1)
        if hwnd is None: raise RuntimeError('test window unavailable')
        time.sleep(3)
        for _ in range(100):
            if picture_aspect(capture(hwnd)): break
            time.sleep(.1)
        else: raise RuntimeError('first imported media did not show')
        time.sleep(1.1) # old build's removed outro must have finished too
        first=capture(hwnd);first.save(work/'first-import.png')
        metrics=[]
        for i in range(24):
            w,h=[(1180,900),(2100,720),(1500,1050),(2400,880)][i%4]
            win32gui.SetWindowPos(hwnd,0,70,60,w,h,win32con.SWP_NOZORDER|win32con.SWP_NOACTIVATE)
            time.sleep(.025)
            img=capture(hwnd); found=picture_aspect(img)
            metrics.append({'frame':i,'aspect':found[0] if found else None})
            if found and abs(found[0]-expected_aspect)>.04: img.save(work/f'distorted-{i}.png')
        time.sleep(.3)
        final=capture(hwnd);final.save(work/'resized.png')
        result={'first':picture_aspect(first),'final':picture_aspect(final)}
        (work/'result.json').write_text(json.dumps({'metrics':metrics,'picture':result},indent=2),encoding='utf-8')
        invalid=[m for m in metrics if m['aspect'] is None or abs(m['aspect']-expected_aspect)>.04]
        print('Native media resize:',json.dumps({'import':'argument','expectedAspect':expected_aspect,'invalidFrames':len(invalid),'frames':len(metrics),'picture':result}))
        if not args.baseline and (invalid or not result['first'] or abs(result['first'][0]-expected_aspect)>.04 or not result['final'] or abs(result['final'][0]-expected_aspect)>.04): raise RuntimeError('native first-frame/resize regression')
    finally:
        # This test never moves the pointer. Do not call SetCursorPos during
        # cleanup: a locked desktop would reject it and strand our process.
        if hwnd: win32gui.PostMessage(hwnd,win32con.WM_CLOSE,0,0)
        try: process.wait(10)
        except subprocess.TimeoutExpired: process.terminate();process.wait()


if __name__=='__main__': main()
