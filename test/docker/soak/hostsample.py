#!/usr/bin/env python3
"""hostsample.py: sample the host's disk and CPU every few seconds into a CSV.

Every node of a soak shares one NVMe and one CPU. This says how busy they are,
so a follower's fill can be compared with the quiet between joins: disk
utilization, IOPS, throughput, per-request wait and queue depth for the whole
device, and CPU user/system/iowait/idle, with the late follower that is up.

Usage: hostsample.py <out.csv> [--device nvme0n1] [--every 10] [--while-pid N]
It stops when the pid given to --while-pid exits (the soak), or on a signal.
"""
import argparse, os, subprocess, sys, time


def diskstats(dev):
    with open("/proc/diskstats") as f:
        for line in f:
            p = line.split()
            if p[2] == dev:
                # reads, sectors read, ms reading, writes, sectors written,
                # ms writing, in flight, ms doing I/O, weighted ms
                return [int(p[i]) for i in (3, 5, 6, 7, 9, 10, 11, 12, 13)]
    raise SystemExit(f"no device {dev} in /proc/diskstats")


def cpustat():
    with open("/proc/stat") as f:
        p = f.readline().split()
    v = [int(x) for x in p[1:9]]  # user nice system idle iowait irq softirq steal
    return v


def follower():
    try:
        out = subprocess.run(["docker", "ps", "--format", "{{.Names}}"], capture_output=True, text=True, timeout=10).stdout
    except Exception:
        return ""
    names = [n for n in out.split() if "-fol" in n and n != "acc-bvn3-fol1"]
    return ";".join(sorted(names))


def alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except OSError:
        return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("out")
    ap.add_argument("--device", default="nvme0n1")
    ap.add_argument("--every", type=float, default=10)
    ap.add_argument("--while-pid", type=int, default=0)
    a = ap.parse_args()
    new = not os.path.exists(a.out)
    f = open(a.out, "a", buffering=1)
    if new:
        f.write("time,util_pct,r_s,w_s,r_MBs,w_MBs,r_await_ms,w_await_ms,queue,cpu_user,cpu_sys,cpu_iowait,cpu_idle,load1,follower\n")
    d0, c0, t0 = diskstats(a.device), cpustat(), time.time()
    while not a.while_pid or alive(a.while_pid):
        time.sleep(a.every)
        d1, c1, t1 = diskstats(a.device), cpustat(), time.time()
        dt = t1 - t0
        dd = [x - y for x, y in zip(d1, d0)]
        dc = [x - y for x, y in zip(c1, c0)]
        rd, rsec, rms, wr, wsec, wms, _, ioms, wtms = dd
        ct = sum(dc) or 1
        row = [
            time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(t1)),
            f"{100 * ioms / (dt * 1000):.1f}",
            f"{rd / dt:.0f}", f"{wr / dt:.0f}",
            f"{rsec * 512 / dt / 1e6:.1f}", f"{wsec * 512 / dt / 1e6:.1f}",
            f"{rms / rd:.2f}" if rd else "0", f"{wms / wr:.2f}" if wr else "0",
            f"{wtms / (dt * 1000):.2f}",
            f"{100 * (dc[0] + dc[1]) / ct:.1f}", f"{100 * (dc[2] + dc[5] + dc[6]) / ct:.1f}",
            f"{100 * dc[4] / ct:.1f}", f"{100 * dc[3] / ct:.1f}",
            f"{os.getloadavg()[0]:.1f}", follower(),
        ]
        f.write(",".join(row) + "\n")
        d0, c0, t0 = d1, c1, t1


if __name__ == "__main__":
    main()
