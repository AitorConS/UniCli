/*
 * Runtime network configuration injection from QEMU fw_cfg.
 *
 * uni's daemon passes "-fw_cfg name=opt/uni/network,string=IP/CIDR,GATEWAY"
 * to QEMU when the user invokes "uni run --ip 10.0.0.2 --network uni-tap0".
 * On boot, before the user program starts, we read that file (if present) and
 * inject static IP configuration into the manifest's root tuple so that
 * init_network_iface() can pick it up and configure the first ethernet
 * interface with a static address instead of DHCP.
 *
 * Format: "IP/CIDR,GATEWAY" (e.g. "10.0.0.2/24,10.0.0.1")
 * Only IPv4 is supported. Safe no-op if the fw_cfg device is absent or the
 * entry is empty.
 */
#ifndef _NET_INJECT_H_
#define _NET_INJECT_H_

#include <unix_internal.h>

void net_inject_from_fw_cfg(tuple root);

#endif