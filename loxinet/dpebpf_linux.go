/*
 * Copyright (c) 2022 NetLOX Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package loxinet

/*
#include <stdio.h>
#include <stdlib.h>
#include <stddef.h>
#include <stdbool.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <assert.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <sys/ioctl.h>
#include <net/if.h>
#include <pthread.h>
#include "../ebpf/kernel/loxilb_libdp.h"
int bpf_map_get_next_key(int fd, const void *key, void *next_key);
int bpf_map_lookup_elem(int fd, const void *key, void *value);
#cgo CFLAGS:  -I./../ebpf/libbpf/src/ -I./../ebpf/common
#cgo LDFLAGS: -L. -L/lib64 -L./../ebpf/kernel -L./../ebpf/libbpf/src/build/usr/lib64/ -Wl,-rpath=/lib64/ -lloxilbdp -lbpf -lelf -lz
*/
import "C"
import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	cmn "github.com/loxilb-io/loxilb/common"
	tk "github.com/loxilb-io/loxilib"
)

// This file implements the interface DpHookInterface
// The implementation is specific to loxilb ebpf datapath for linux

// error codes
const (
	EbpfErrBase = iota - 50000
	EbpfErrPortPropAdd
	EbpfErrPortPropDel
	EbpfErrEbpfLoad
	EbpfErrEbpfUnload
	EbpfErrL2AddrAdd
	EbpfErrL2AddrDel
	EbpfErrTmacAdd
	EbpfErrTmacDel
	EbpfErrNhAdd
	EbpfErrNhDel
	EbpfErrRt4Add
	EbpfErrRt4Del
	EbpfErrNat4Add
	EbpfErrNat4Del
	EbpfErrSess4Add
	EbpfErrSess4Del
	EbpfErrPolAdd
	EbpfErrPolDel
	EbpfErrMirrAdd
	EbpfErrMirrDel
	EbpfErrWqUnk
)

// constants
const (
	DpEbpfLinuxTiVal = 20
)

// ebpf table related defines in go
type (
	sActValue   C.struct_dp_cmn_act
	intfMapKey  C.struct_intf_key
	intfMapDat  C.struct_dp_intf_tact
	intfSetIfi  C.struct_dp_intf_tact_set_ifi
	sMacKey     C.struct_dp_smac_key
	dMacKey     C.struct_dp_dmac_key
	dMacMapDat  C.struct_dp_dmac_tact
	l2VlanAct   C.struct_dp_l2vlan_act
	tMacKey     C.struct_dp_tmac_key
	tMacDat     C.struct_dp_tmac_tact
	rtNhAct     C.struct_dp_rt_nh_act
	nhKey       C.struct_dp_nh_key
	nhDat       C.struct_dp_nh_tact
	rtL2NhAct   C.struct_dp_rt_l2nh_act
	rtVxL2NhAct C.struct_dp_rt_l2vxnh_act
	rt4Key      C.struct_dp_rtv4_key
	rtDat       C.struct_dp_rt_tact
	rtL3NhAct   C.struct_dp_rt_nh_act
	nat4Key     C.struct_dp_natv4_key
	nat4Acts    C.struct_dp_natv4_tacts
	nxfrmAct    C.struct_mf_xfrm_inf
	sess4Key    C.struct_dp_sess4_key
	sessAct     C.struct_dp_sess_tact
	polTact     C.struct_dp_pol_tact
	polAct      C.struct_dp_policer_act
	mirrTact    C.struct_dp_mirr_tact
)

// DpEbpfH - context container
type DpEbpfH struct {
	ticker *time.Ticker
	tDone  chan bool
	tbN    int
}

// dpEbpfTicker - this ticker routine runs every DPEBPF_LINUX_TIVAL seconds
func dpEbpfTicker() {
	tbls := []int{int(C.LL_DP_RTV4_STATS_MAP),
		int(C.LL_DP_TMAC_STATS_MAP),
		int(C.LL_DP_BD_STATS_MAP),
		int(C.LL_DP_TX_BD_STATS_MAP),
		int(C.LL_DP_SESS4_STATS_MAP)}
	tLen := len(tbls)

	for {
		if mh.dpEbpf == nil {
			continue
		}
		select {
		case <-mh.dpEbpf.tDone:
			return
		case t := <-mh.dpEbpf.ticker.C:
			sel := mh.dpEbpf.tbN % tLen
			tk.LogIt(-1, "DP Tick at for selector %v:%d\n", t, sel)

			// For every tick collect stats for an eBPF map
			// This routine caches stats in a local statsDB
			// which can be collected from a separate gothread
			C.llb_collect_map_stats(C.int(tbls[sel]))

			// Age any entries related to Conntrack
			// Conntrack entries also use ACL entries for fast-forwarding
			// which might also get aged out in this process
			C.llb_collect_map_stats(C.int(C.LL_DP_ACLV4_STATS_MAP))
			C.llb_age_map_entries(C.LL_DP_CTV4_MAP)
			C.llb_age_map_entries(C.LL_DP_FCV4_MAP)
			mh.dpEbpf.tbN++
		}
	}
}

// DpEbpfInit - initialize the ebpf dp subsystem
func DpEbpfInit() *DpEbpfH {
	C.loxilb_main()

	// Make sure to unload eBPF programs at init time
	ifList, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, intf := range ifList {
		if intf.Name == "llb0" {
			continue
		}
		tk.LogIt(tk.LogInfo, "ebpf unload - %s\n", intf.Name)
		ifStr := C.CString(intf.Name)
		section := C.CString(string(C.TC_LL_SEC_DEFAULT))
		C.llb_dp_link_attach(ifStr, section, C.LL_BPF_MOUNT_TC, 1)
		C.free(unsafe.Pointer(ifStr))
		C.free(unsafe.Pointer(section))
	}

	ne := new(DpEbpfH)
	ne.tDone = make(chan bool)
	ne.ticker = time.NewTicker(DpEbpfLinuxTiVal * time.Second)

	go dpEbpfTicker()

	return ne
}

// loadEbpfPgm - load loxilb eBPF program to an interface
func loadEbpfPgm(name string) int {
	ifStr := C.CString(name)
	section := C.CString(string(C.TC_LL_SEC_DEFAULT))
	ret := C.llb_dp_link_attach(ifStr, section, C.LL_BPF_MOUNT_TC, 0)
	C.free(unsafe.Pointer(ifStr))
	C.free(unsafe.Pointer(section))
	return int(ret)
}

// unLoadEbpfPgm - unload loxilb eBPF program from an interface
func unLoadEbpfPgm(name string) int {
	ifStr := C.CString(name)
	section := C.CString(string(C.XDP_LL_SEC_DEFAULT))
	ret := C.llb_dp_link_attach(ifStr, section, C.LL_BPF_MOUNT_TC, 1)
	C.free(unsafe.Pointer(ifStr))
	C.free(unsafe.Pointer(section))
	return int(ret)
}

func getPtrOffset(ptr unsafe.Pointer, size uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(ptr) + size)
}

func osPortIsRunning(portName string) bool {
	sfd, err := syscall.Socket(syscall.AF_INET,
		syscall.SOCK_DGRAM,
		syscall.IPPROTO_IP)
	if err != nil {
		tk.LogIt(tk.LogError, "Error %s", err)
		return false
	}

	ifstr := C.CString(portName)
	ifrStruct := make([]byte, 32)
	C.memcpy(unsafe.Pointer(&ifrStruct[0]), unsafe.Pointer(ifstr), 16)

	r0, _, err := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(sfd),
		syscall.SIOCGIFFLAGS,
		uintptr(unsafe.Pointer(&ifrStruct[0])))
	if r0 != 0 {
		C.free(unsafe.Pointer(ifstr))
		syscall.Close(sfd)
		tk.LogIt(tk.LogError, "Error %s", err)
		return false
	}

	C.free(unsafe.Pointer(ifstr))
	syscall.Close(sfd)

	var flags uint16
	C.memcpy(unsafe.Pointer(&flags), unsafe.Pointer(&ifrStruct[16]), 2)

	if flags&syscall.IFF_RUNNING != 0 {
		return true
	}

	return false
}

// DpPortPropMod - routine to work on a ebpf port property request
func DpPortPropMod(w *PortDpWorkQ) int {
	var txK C.uint
	var txV C.uint
	var setIfi *intfSetIfi

	// This is a special case
	if w.LoadEbpf == "llb0" {
		w.PortNum = C.LLB_INTERFACES - 1
	}

	key := new(intfMapKey)
	key.ing_vid = C.ushort(tk.Htons(uint16(w.IngVlan)))
	key.ifindex = C.uint(w.OsPortNum)

	txK = C.uint(w.PortNum)

	if w.Work == DpCreate {

		if w.LoadEbpf != "" && w.LoadEbpf != "lo" && w.LoadEbpf != "llb0" {
			lRet := loadEbpfPgm(w.LoadEbpf)
			if lRet != 0 {
				tk.LogIt(tk.LogError, "ebpf load - %d error\n", w.PortNum)
				return EbpfErrEbpfLoad
			}
		}
		data := new(intfMapDat)
		C.memset(unsafe.Pointer(data), 0, C.sizeof_struct_dp_intf_tact)
		data.ca.act_type = C.DP_SET_IFI
		setIfi = (*intfSetIfi)(getPtrOffset(unsafe.Pointer(data),
			C.sizeof_struct_dp_cmn_act))

		setIfi.xdp_ifidx = C.ushort(w.PortNum)
		setIfi.zone = C.ushort(w.SetZoneNum)

		setIfi.bd = C.ushort(uint16(w.SetBD))
		setIfi.mirr = C.ushort(w.SetMirr)
		setIfi.polid = C.ushort(w.SetPol)

		if w.Prop&cmn.PortPropUpp == cmn.PortPropUpp {
			setIfi.pprop = C.LLB_DP_PORT_UPP
		}

		ret := C.llb_add_map_elem(C.LL_DP_INTF_MAP, unsafe.Pointer(key), unsafe.Pointer(data))

		if ret != 0 {
			tk.LogIt(tk.LogError, "ebpf intfmap - %d vlan %d error\n", w.OsPortNum, w.IngVlan)
			return EbpfErrPortPropAdd
		}

		tk.LogIt(tk.LogDebug, "ebpf intfmap added - %d vlan %d -> %d\n", w.OsPortNum, w.IngVlan, w.PortNum)

		txV = C.uint(w.OsPortNum)
		ret = C.llb_add_map_elem(C.LL_DP_TX_INTF_MAP, unsafe.Pointer(&txK), unsafe.Pointer(&txV))
		if ret != 0 {
			C.llb_del_map_elem(C.LL_DP_INTF_MAP, unsafe.Pointer(key))
			tk.LogIt(tk.LogError, "ebpf txintfmap - %d error\n", w.OsPortNum)
			return EbpfErrPortPropAdd
		}
		tk.LogIt(tk.LogDebug, "ebpf txintfmap added - %d -> %d\n", w.PortNum, w.OsPortNum)
		return 0
	} else if w.Work == DpRemove {

		// TX_INTF_MAP is array type so we can't delete it
		// Rather we need to zero it out first
		txV = C.uint(0)
		C.llb_add_map_elem(C.LL_DP_TX_INTF_MAP, unsafe.Pointer(&txK), unsafe.Pointer(&txV))
		C.llb_del_map_elem(C.LL_DP_TX_INTF_MAP, unsafe.Pointer(&txK))

		C.llb_del_map_elem(C.LL_DP_INTF_MAP, unsafe.Pointer(key))

		if w.LoadEbpf != "" {
			lRet := unLoadEbpfPgm(w.LoadEbpf)
			if lRet != 0 {
				tk.LogIt(tk.LogError, "ebpf unload - ifi %d error\n", w.OsPortNum)
				return EbpfErrEbpfLoad
			}
			tk.LogIt(tk.LogDebug, "ebpf unloaded - ifi %d\n", w.OsPortNum)
		}

		return 0
	}

	return EbpfErrWqUnk
}

// DpPortPropAdd - routine to work on a ebpf port property add
func (e *DpEbpfH) DpPortPropAdd(w *PortDpWorkQ) int {
	return DpPortPropMod(w)
}

// DpPortPropDel - routine to work on a ebpf port property delete
func (e *DpEbpfH) DpPortPropDel(w *PortDpWorkQ) int {
	return DpPortPropMod(w)
}

// DpL2AddrMod - routine to work on a ebpf l2 addr request
func DpL2AddrMod(w *L2AddrDpWorkQ) int {
	var l2va *l2VlanAct

	skey := new(sMacKey)
	C.memcpy(unsafe.Pointer(&skey.smac[0]), unsafe.Pointer(&w.L2Addr[0]), 6)
	skey.bd = C.ushort((uint16(w.BD)))

	dkey := new(dMacKey)
	C.memcpy(unsafe.Pointer(&dkey.dmac[0]), unsafe.Pointer(&w.L2Addr[0]), 6)
	dkey.bd = C.ushort((uint16(w.BD)))

	if w.Work == DpCreate {
		sdat := new(sActValue)
		sdat.act_type = C.DP_SET_NOP

		ddat := new(dMacMapDat)
		C.memset(unsafe.Pointer(ddat), 0, C.sizeof_struct_dp_dmac_tact)

		if w.Tun == 0 {
			l2va = (*l2VlanAct)(getPtrOffset(unsafe.Pointer(ddat),
				C.sizeof_struct_dp_cmn_act))
			if w.Tagged != 0 {
				ddat.ca.act_type = C.DP_SET_ADD_L2VLAN
				l2va.vlan = C.ushort(tk.Htons(uint16(w.BD)))
				l2va.oport = C.ushort(w.PortNum)
			} else {
				ddat.ca.act_type = C.DP_SET_RM_L2VLAN
				l2va.vlan = C.ushort(tk.Htons(uint16(w.BD)))
				l2va.oport = C.ushort(w.PortNum)
			}
		}

		ret := C.llb_add_map_elem(C.LL_DP_SMAC_MAP,
			unsafe.Pointer(skey),
			unsafe.Pointer(sdat))
		if ret != 0 {
			return EbpfErrL2AddrAdd
		}

		if w.Tun == 0 {
			ret = C.llb_add_map_elem(C.LL_DP_DMAC_MAP,
				unsafe.Pointer(dkey),
				unsafe.Pointer(ddat))
			if ret != 0 {
				C.llb_del_map_elem(C.LL_DP_SMAC_MAP, unsafe.Pointer(skey))
				return EbpfErrL2AddrAdd
			}
		}

		return 0
	} else if w.Work == DpRemove {

		C.llb_del_map_elem(C.LL_DP_SMAC_MAP, unsafe.Pointer(skey))

		if w.Tun == 0 {
			C.llb_del_map_elem(C.LL_DP_DMAC_MAP, unsafe.Pointer(dkey))
		}

		return 0
	}

	return EbpfErrWqUnk
}

// DpL2AddrAdd - routine to work on a ebpf l2 addr add
func (e *DpEbpfH) DpL2AddrAdd(w *L2AddrDpWorkQ) int {
	return DpL2AddrMod(w)
}

// DpL2AddrDel - routine to work on a ebpf l2 addr delete
func (e *DpEbpfH) DpL2AddrDel(w *L2AddrDpWorkQ) int {
	return DpL2AddrMod(w)
}

// DpRouterMacMod - routine to work on a ebpf rt-mac change request
func DpRouterMacMod(w *RouterMacDpWorkQ) int {

	key := new(tMacKey)
	C.memcpy(unsafe.Pointer(&key.mac[0]), unsafe.Pointer(&w.L2Addr[0]), 6)
	switch {
	case w.TunType == DpTunVxlan:
		key.tun_type = C.LLB_TUN_VXLAN
	case w.TunType == DpTunGre:
		key.tun_type = C.LLB_TUN_GRE
	case w.TunType == DpTunGtp:
		key.tun_type = C.LLB_TUN_GTP
	case w.TunType == DpTunStt:
		key.tun_type = C.LLB_TUN_STT
	}

	key.tunnel_id = C.uint(w.TunID)

	if w.Work == DpCreate {
		dat := new(sActValue)
		if w.TunID != 0 {
			if w.NhNum == 0 {
				dat.act_type = C.DP_SET_RM_VXLAN
				rtNhAct := (*rtNhAct)(getPtrOffset(unsafe.Pointer(dat),
					C.sizeof_struct_dp_cmn_act))
				C.memset(unsafe.Pointer(rtNhAct), 0, C.sizeof_struct_dp_rt_nh_act)
				rtNhAct.nh_num = 0
				rtNhAct.tid = 0
				rtNhAct.bd = C.ushort(w.BD)
			} else {
				/* No need for tunnel ID in case of Access side */
				key.tunnel_id = 0
				key.tun_type = 0
				dat.act_type = C.DP_SET_RT_TUN_NH
				rtNhAct := (*rtNhAct)(getPtrOffset(unsafe.Pointer(dat),
					C.sizeof_struct_dp_cmn_act))
				C.memset(unsafe.Pointer(rtNhAct), 0, C.sizeof_struct_dp_rt_nh_act)

				rtNhAct.nh_num = C.ushort(w.NhNum)
				tid := ((w.TunID << 8) & 0xffffff00)
				rtNhAct.tid = C.uint(tk.Htonl(tid))
			}
		} else {
			dat.act_type = C.DP_SET_L3_EN
		}

		ret := C.llb_add_map_elem(C.LL_DP_TMAC_MAP,
			unsafe.Pointer(key),
			unsafe.Pointer(dat))

		if ret != 0 {
			return EbpfErrTmacAdd
		}

		return 0
	} else if w.Work == DpRemove {

		C.llb_del_map_elem(C.LL_DP_TMAC_MAP, unsafe.Pointer(key))
	}

	return EbpfErrWqUnk
}

// DpRouterMacAdd - routine to work on a ebpf rt-mac add request
func (e *DpEbpfH) DpRouterMacAdd(w *RouterMacDpWorkQ) int {
	return DpRouterMacMod(w)
}

// DpRouterMacDel - routine to work on a ebpf rt-mac delete request
func (e *DpEbpfH) DpRouterMacDel(w *RouterMacDpWorkQ) int {
	return DpRouterMacMod(w)
}

// DpNextHopMod - routine to work on a ebpf next-hop change request
func DpNextHopMod(w *NextHopDpWorkQ) int {
	var act *rtL2NhAct
	var vxAct *rtVxL2NhAct

	key := new(nhKey)
	key.nh_num = C.uint(w.NextHopNum)

	if w.Work == DpCreate {
		dat := new(nhDat)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_nh_tact)
		if !w.Resolved {
			dat.ca.act_type = C.DP_SET_TOCP
		} else {
			if w.TunNh {
				tk.LogIt(tk.LogDebug, "Setting tunNh %x\n", key.nh_num)
				dat.ca.act_type = C.DP_SET_NEIGH_VXLAN
				vxAct = (*rtVxL2NhAct)(getPtrOffset(unsafe.Pointer(dat),
					C.sizeof_struct_dp_cmn_act))

				ipAddr := tk.IPtonl(w.RIP)
				vxAct.l3t.rip = C.uint(ipAddr)
				vxAct.l3t.sip = C.uint(tk.IPtonl(w.SIP))
				tid := ((w.TunID << 8) & 0xffffff00)
				vxAct.l3t.tid = C.uint(tk.Htonl(tid))

				tk.LogIt(tk.LogDebug, "rip 0x%x sip 0x%x 0x%x\n", vxAct.l3t.sip, vxAct.l3t.rip, vxAct.l3t.tid)

				act = (*rtL2NhAct)(&vxAct.l2nh)
				C.memcpy(unsafe.Pointer(&act.dmac[0]), unsafe.Pointer(&w.DstAddr[0]), 6)
				C.memcpy(unsafe.Pointer(&act.smac[0]), unsafe.Pointer(&w.SrcAddr[0]), 6)
				act.bd = C.ushort(w.BD)
			} else {
				dat.ca.act_type = C.DP_SET_NEIGH_L2
				act = (*rtL2NhAct)(getPtrOffset(unsafe.Pointer(dat),
					C.sizeof_struct_dp_cmn_act))
				C.memcpy(unsafe.Pointer(&act.dmac[0]), unsafe.Pointer(&w.DstAddr[0]), 6)
				C.memcpy(unsafe.Pointer(&act.smac[0]), unsafe.Pointer(&w.SrcAddr[0]), 6)
				act.bd = C.ushort(w.BD)
				act.rnh_num = C.ushort(w.NNextHopNum)
			}
		}

		ret := C.llb_add_map_elem(C.LL_DP_NH_MAP,
			unsafe.Pointer(key),
			unsafe.Pointer(dat))
		if ret != 0 {
			return EbpfErrNhAdd
		}
		return 0
	} else if w.Work == DpRemove {
		dat := new(nhDat)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_nh_tact)
		//C.llb_del_table_elem(C.LL_DP_NH_MAP, unsafe.Pointer(key))
		// eBPF array elements cant be delete. Instead we just reset it
		C.llb_add_map_elem(C.LL_DP_NH_MAP,
			unsafe.Pointer(key),
			unsafe.Pointer(dat))
		return 0
	}

	return EbpfErrWqUnk
}

// DpNextHopAdd - routine to work on a ebpf next-hop add request
func (e *DpEbpfH) DpNextHopAdd(w *NextHopDpWorkQ) int {
	return DpNextHopMod(w)
}

// DpNextHopDel - routine to work on a ebpf next-hop delete request
func (e *DpEbpfH) DpNextHopDel(w *NextHopDpWorkQ) int {
	return DpNextHopMod(w)
}

// DpRouteMod - routine to work on a ebpf route change request
func DpRouteMod(w *RouteDpWorkQ) int {
	var act *rtL3NhAct
	var kPtr *[6]uint8

	key := new(rt4Key)

	len, _ := w.Dst.Mask.Size()
	len += 16 /* 16-bit ZoneNum + prefix-len */
	key.l.prefixlen = C.uint(len)
	kPtr = (*[6]uint8)(getPtrOffset(unsafe.Pointer(key),
		C.sizeof_struct_bpf_lpm_trie_key))

	kPtr[0] = uint8(w.ZoneNum >> 8 & 0xff)
	kPtr[1] = uint8(w.ZoneNum & 0xff)
	kPtr[2] = uint8(w.Dst.IP[0])
	kPtr[3] = uint8(w.Dst.IP[1])
	kPtr[4] = uint8(w.Dst.IP[2])
	kPtr[5] = uint8(w.Dst.IP[3])

	if w.Work == DpCreate {
		dat := new(rtDat)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_rt_tact)

		if w.NHwMark >= 0 {
			dat.ca.act_type = C.DP_SET_RT_NHNUM
			act = (*rtL3NhAct)(getPtrOffset(unsafe.Pointer(dat),
				C.sizeof_struct_dp_cmn_act))
			act.nh_num = C.ushort(w.NHwMark)
		} else {
			dat.ca.act_type = C.DP_SET_TOCP
		}

		if w.RtHwMark > 0 {
			dat.ca.cidx = C.uint(w.RtHwMark)
		}

		ret := C.llb_add_map_elem(C.LL_DP_RTV4_MAP,
			unsafe.Pointer(key),
			unsafe.Pointer(dat))
		if ret != 0 {
			return EbpfErrRt4Add
		}
		return 0
	} else if w.Work == DpRemove {
		C.llb_del_map_elem(C.LL_DP_RTV4_MAP, unsafe.Pointer(key))

		if w.RtHwMark > 0 {
			C.llb_clear_map_stats(C.LL_DP_RTV4_STATS_MAP, C.uint(w.RtHwMark))
		}
		return 0
	}

	return EbpfErrWqUnk
}

// DpRouteAdd - routine to work on a ebpf route add request
func (e *DpEbpfH) DpRouteAdd(w *RouteDpWorkQ) int {
	return DpRouteMod(w)
}

// DpRouteDel - routine to work on a ebpf route delete request
func (e *DpEbpfH) DpRouteDel(w *RouteDpWorkQ) int {
	return DpRouteMod(w)
}

// DpNatLbRuleMod - routine to work on a ebpf nat-lb change request
func DpNatLbRuleMod(w *NatDpWorkQ) int {

	key := new(nat4Key)

	key.daddr = C.uint(tk.IPtonl(w.ServiceIP))
	key.dport = C.ushort(tk.Htons(w.L4Port))
	key.l4proto = C.uchar(w.Proto)
	key.zone = C.ushort(w.ZoneNum)

	if w.Work == DpCreate {
		dat := new(nat4Acts)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_natv4_tacts)
		if w.NatType == DpSnat {
			dat.ca.act_type = C.DP_SET_SNAT
		} else if w.NatType == DpDnat {
			dat.ca.act_type = C.DP_SET_DNAT
		} else {
			return EbpfErrNat4Add
		}

		switch {
		case w.EpSel == EpRR:
			dat.sel_type = C.NAT_LB_SEL_RR
		case w.EpSel == EpHash:
			dat.sel_type = C.NAT_LB_SEL_HASH
		/* Currently not implemented in DP */
		/*case w.EpSel == EP_PRIO:
		  dat.sel_type = C.NAT_LB_SEL_PRIO*/
		default:
			dat.sel_type = C.NAT_LB_SEL_RR
		}
		dat.ca.cidx = C.uint(w.HwMark)

		nxfa := (*nxfrmAct)(unsafe.Pointer(&dat.nxfrms[0]))

		for _, k := range w.endPoints {
			nxfa.wprio = C.ushort(k.Weight)
			nxfa.nat_xport = C.ushort(tk.Htons(k.XPort))
			nxfa.nat_xip = C.uint(tk.IPtonl(k.XIP))

			if k.InActive {
				nxfa.inactive = 1
			}

			nxfa = (*nxfrmAct)(getPtrOffset(unsafe.Pointer(nxfa),
				C.sizeof_struct_mf_xfrm_inf))
		}

		// Any unused end-points should be marked inactive
		for i := len(w.endPoints); i < C.LLB_MAX_NXFRMS; i++ {
			nxfa := (*nxfrmAct)(unsafe.Pointer(&dat.nxfrms[i]))
			nxfa.inactive = 1
		}

		dat.nxfrm = C.uint(len(w.endPoints))

		ret := C.llb_add_map_elem(C.LL_DP_NAT4_MAP,
			unsafe.Pointer(key),
			unsafe.Pointer(dat))

		if ret != 0 {
			return EbpfErrTmacAdd
		}

		return 0
	} else if w.Work == DpRemove {
		C.llb_del_map_elem(C.LL_DP_NAT4_MAP, unsafe.Pointer(key))
		return 0
	}

	return EbpfErrWqUnk
}

// DpNatLbRuleAdd - routine to work on a ebpf nat-lb add request
func (e *DpEbpfH) DpNatLbRuleAdd(w *NatDpWorkQ) int {
	return DpNatLbRuleMod(w)
}

// DpNatLbRuleDel - routine to work on a ebpf nat-lb delete request
func (e *DpEbpfH) DpNatLbRuleDel(w *NatDpWorkQ) int {
	return DpNatLbRuleMod(w)
}

// DpStat - routine to work on a ebpf map statistics request
func (e *DpEbpfH) DpStat(w *StatDpWorkQ) int {
	var packets, bytes, dropPackets uint64
	var tbl []int
	var polTbl []int
	sync := 0
	switch {
	case w.Name == MapNameNat4:
		tbl = append(tbl, int(C.LL_DP_NAT4_STATS_MAP))
		sync = 1
	case w.Name == MapNameBD:
		tbl = append(tbl, int(C.LL_DP_BD_STATS_MAP), int(C.LL_DP_TX_BD_STATS_MAP))
	case w.Name == MapNameRxBD:
		tbl = append(tbl, int(C.LL_DP_BD_STATS_MAP))
	case w.Name == MapNameTxBD:
		tbl = append(tbl, int(C.LL_DP_TX_BD_STATS_MAP))
	case w.Name == MapNameRt4:
		tbl = append(tbl, int(C.LL_DP_RTV4_MAP))
	case w.Name == MapNameULCL:
		tbl = append(tbl, int(C.LL_DP_SESS4_MAP))
	case w.Name == MapNameIpol:
		polTbl = append(polTbl, int(C.LL_DP_POL_MAP))
	default:
		return EbpfErrWqUnk
	}

	if w.Work == DpStatsGet {
		var b C.longlong
		var p C.longlong

		packets = 0
		bytes = 0
		dropPackets = 0

		for _, t := range tbl {

			ret := C.llb_fetch_map_stats_cached(C.int(t), C.uint(w.HwMark), C.int(sync),
				(unsafe.Pointer(&b)), unsafe.Pointer(&p))
			if ret != 0 {
				return EbpfErrTmacAdd
			}

			packets += uint64(p)
			bytes += uint64(b)
		}

		for _, t := range polTbl {

			ret := C.llb_fetch_pol_map_stats(C.int(t), C.uint(w.HwMark), (unsafe.Pointer(&p)), unsafe.Pointer(&b))
			if ret != 0 {
				return EbpfErrTmacAdd
			}

			packets += uint64(p)
			dropPackets += uint64(b)
		}

		if packets != 0 || bytes != 0 || dropPackets != 0 {
			if w.Packets != nil {
				*w.Packets = uint64(packets)
			}
			if w.Bytes != nil {
				*w.Bytes = uint64(bytes)
			}
			if w.DropPackets != nil {
				*w.DropPackets = uint64(dropPackets)
			}
		}
	} else if w.Work == DpStatsClr {
		for _, t := range tbl {
			C.llb_clear_map_stats(C.int(t), C.uint(w.HwMark))
		}
	}

	return 0
}

func convDPCt2GoObj(ctKey *C.struct_dp_ctv4_key, ctDat *C.struct_dp_ctv4_dat) *DpCtInfo {
	ct := new(DpCtInfo)

	ct.DIP = tk.NltoIP(uint32(ctKey.daddr))
	ct.SIP = tk.NltoIP(uint32(ctKey.saddr))
	ct.Dport = tk.Ntohs(uint16(ctKey.dport))
	ct.Sport = tk.Ntohs(uint16(ctKey.sport))
	ct.Packets = uint64(ctDat.pb.packets)
	ct.Bytes = uint64(ctDat.pb.bytes)

	p := uint8(ctKey.l4proto)
	switch {
	case p == 1:
		ct.Proto = "icmp"
		i := (*C.ct_icmp_pinf_t)(unsafe.Pointer(&ctDat.pi))
		switch {
		case i.state&C.CT_ICMP_DUNR != 0:
			ct.CState = "dest-unr"
		case i.state&C.CT_ICMP_TTL != 0:
			ct.CState = "ttl-exp"
		case i.state&C.CT_ICMP_RDR != 0:
			ct.CState = "icmp-redir"
		case i.state == C.CT_ICMP_CLOSED:
			ct.CState = "closed"
		case i.state == C.CT_ICMP_REQS:
			ct.CState = "req-sent"
		case i.state == C.CT_ICMP_REPS:
			ct.CState = "bidir"
		}
	case p == 6:
		ct.Proto = "tcp"
		t := (*C.ct_tcp_pinf_t)(unsafe.Pointer(&ctDat.pi))
		switch {
		case t.state == C.CT_TCP_CLOSED:
			ct.CState = "closed"
		case t.state == C.CT_TCP_SS:
			ct.CState = "sync-sent"
		case t.state == C.CT_TCP_SA:
			ct.CState = "sync-ack"
		case t.state == C.CT_TCP_EST:
			ct.CState = "est"
		case t.state == C.CT_TCP_ERR:
			ct.CState = "h/e"
		case t.state == C.CT_TCP_CW:
			ct.CState = "closed-wait"
		default:
			ct.CState = "fini"
		}
	case p == 17:
		ct.Proto = "udp"
		u := (*C.ct_udp_pinf_t)(unsafe.Pointer(&ctDat.pi))
		switch {
		case u.state == C.CT_UDP_CNI:
			ct.CState = "closed"
		case u.state == C.CT_UDP_UEST:
			ct.CState = "udp-uni"
		case u.state == C.CT_UDP_EST:
			ct.CState = "udp-est"
		default:
			ct.CState = "unk"
		}
	case p == 132:
		ct.Proto = "sctp"
		s := (*C.ct_sctp_pinf_t)(unsafe.Pointer(&ctDat.pi))
		switch {
		case s.state == C.CT_SCTP_EST:
			ct.CState = "est"
		case s.state == C.CT_SCTP_CLOSED:
			ct.CState = "closed"
		case s.state == C.CT_SCTP_ERR:
			ct.CState = "err"
		case s.state == C.CT_SCTP_INIT:
			ct.CState = "init"
		case s.state == C.CT_SCTP_INITA:
			ct.CState = "init-ack"
		case s.state == C.CT_SCTP_COOKIE:
			ct.CState = "cookie-echo"
		case s.state == C.CT_SCTP_COOKIEA:
			ct.CState = "cookie-echo-resp"
		case s.state == C.CT_SCTP_SHUT:
			ct.CState = "shut"
		case s.state == C.CT_SCTP_SHUTA:
			ct.CState = "shut-ack"
		case s.state == C.CT_SCTP_SHUTC:
			ct.CState = "shut-complete"
		case s.state == C.CT_SCTP_ABRT:
			ct.CState = "abort"
		default:
			ct.CState = "unk"
		}
	default:
		ct.Proto = fmt.Sprintf("%d", p)
	}

	if ctDat.xi.nat_flags == C.LLB_NAT_DST ||
		ctDat.xi.nat_flags == C.LLB_NAT_SRC {
		var xip net.IP

		xip = append(xip, uint8(ctDat.xi.nat_xip&0xff))
		xip = append(xip, uint8(ctDat.xi.nat_xip>>8&0xff))
		xip = append(xip, uint8(ctDat.xi.nat_xip>>16&0xff))
		xip = append(xip, uint8(ctDat.xi.nat_xip>>24&0xff))

		port := tk.Ntohs(uint16(ctDat.xi.nat_xport))

		if ctDat.xi.nat_flags == C.LLB_NAT_DST {
			ct.CAct = fmt.Sprintf("dnat-%s:%d:w%d", xip.String(), port, ctDat.xi.wprio)
		} else if ctDat.xi.nat_flags == C.LLB_NAT_SRC {
			ct.CAct = fmt.Sprintf("snat-%s:%d:w%d", xip.String(), port, ctDat.xi.wprio)
		}
	}

	return ct
}

// DpTableGet - routine to work on a ebpf map get request
func (e *DpEbpfH) DpTableGet(w *TableDpWorkQ) (DpRetT, error) {
	var tbl int

	if w.Work != DpMapGet {
		return EbpfErrWqUnk, errors.New("unknown work type")
	}

	switch {
	case w.Name == MapNameCt4:
		tbl = C.LL_DP_ACLV4_MAP
	default:
		return EbpfErrWqUnk, errors.New("unknown work type")
	}

	if tbl == C.LL_DP_ACLV4_MAP {
		ctMap := make(map[string]*DpCtInfo)
		var n int = 0
		var key *C.struct_dp_ctv4_key = nil
		nextKey := new(C.struct_dp_ctv4_key)
		var tact C.struct_dp_aclv4_tact
		var act *C.struct_dp_ctv4_dat

		fd := C.llb_map2fd(C.int(tbl))

		for C.bpf_map_get_next_key(C.int(fd), (unsafe.Pointer)(key), (unsafe.Pointer)(nextKey)) == 0 {
			ctKey := (*C.struct_dp_ctv4_key)(unsafe.Pointer(nextKey))

			if C.bpf_map_lookup_elem(C.int(fd), (unsafe.Pointer)(nextKey), (unsafe.Pointer)(&tact)) != 0 {
				continue
			}

			act = &tact.ctd

			if act.dir == C.CT_DIR_IN || act.dir == C.CT_DIR_OUT {
				var b, p uint64
				goCt4Ent := convDPCt2GoObj(ctKey, act)
				ret := C.llb_fetch_map_stats_cached(C.int(C.LL_DP_ACLV4_STATS_MAP), C.uint(tact.ca.cidx), C.int(1),
					(unsafe.Pointer(&b)), unsafe.Pointer(&p))
				if ret == 0 {
					goCt4Ent.Bytes += b
					goCt4Ent.Packets += p
				}
				fmt.Println(goCt4Ent)
				ctMap[goCt4Ent.Key()] = goCt4Ent
			}
			key = nextKey
			n++
		}
		return ctMap, nil
	}

	return EbpfErrWqUnk, errors.New("unknown work type")
}

// DpUlClMod - routine to work on a ebpf ul-cl filter change request
func (e *DpEbpfH) DpUlClMod(w *UlClDpWorkQ) int {
	key := new(sess4Key)

	key.daddr = C.uint(tk.IPtonl(w.MDip))
	key.saddr = C.uint(tk.IPtonl(w.MSip))
	key.teid = C.uint(tk.Htonl(w.mTeID))
	key.r = 0

	if w.Work == DpCreate {
		dat := new(sessAct)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_sess_tact)

		if key.teid != 0 {
			dat.ca.act_type = C.DP_SET_RM_GTP
			dat.ca.cidx = C.uint(w.HwMark)
			dat.qfi = C.uchar(w.Qfi)
		} else {
			dat.ca.act_type = C.DP_SET_ADD_GTP
			dat.ca.cidx = C.uint(w.HwMark)
			dat.qfi = C.uchar(w.Qfi)
			dat.rip = C.uint(tk.IPtonl(w.TDip))
			dat.sip = C.uint(tk.IPtonl(w.TSip))
			dat.teid = C.uint(tk.Htonl(w.TTeID))
		}

		ret := C.llb_add_map_elem(C.LL_DP_SESS4_MAP,
			unsafe.Pointer(key),
			unsafe.Pointer(dat))

		if ret != 0 {
			return EbpfErrSess4Add
		}

		return 0
	} else if w.Work == DpRemove {
		C.llb_del_map_elem(C.LL_DP_SESS4_MAP, unsafe.Pointer(key))
		return 0
	}
	return EbpfErrWqUnk
}

// DpUlClAdd - routine to work on a ebpf ul-cl filter add request
func (e *DpEbpfH) DpUlClAdd(w *UlClDpWorkQ) int {
	return e.DpUlClMod(w)
}

// DpUlClDel - routine to work on a ebpf ul-cl filter delete request
func (e *DpEbpfH) DpUlClDel(w *UlClDpWorkQ) int {
	return e.DpUlClMod(w)
}

// DpPolMod - routine to work on a ebpf policer change request
func (e *DpEbpfH) DpPolMod(w *PolDpWorkQ) int {
	key := C.uint(w.HwMark)

	if w.Work == DpCreate {
		dat := new(polTact)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_pol_tact)
		dat.ca.act_type = C.DP_SET_DO_POLICER
		// For finding pa, we need to account for padding of 4
		pa := (*polAct)(getPtrOffset(unsafe.Pointer(dat),
			C.sizeof_struct_dp_cmn_act+C.sizeof_struct_bpf_spin_lock+4))

		if w.Srt == false {
			pa.trtcm = 1
		} else {
			pa.trtcm = 0
		}

		if w.Color == false {
			pa.color_aware = 0
		} else {
			pa.color_aware = 1
		}

		pa.toksc_pus = C.ulonglong(w.Cir / (8000000))
		pa.tokse_pus = C.ulonglong(w.Pir / (8000000))
		pa.cbs = C.uint(w.Cbs)
		pa.ebs = C.uint(w.Ebs)
		pa.tok_c = pa.cbs
		pa.tok_e = pa.ebs
		pa.lastc_uts = C.get_os_usecs()
		pa.laste_uts = pa.toksc_pus
		pa.drop_prio = C.LLB_PIPE_COL_YELLOW

		ret := C.llb_add_map_elem(C.LL_DP_POL_MAP,
			unsafe.Pointer(&key),
			unsafe.Pointer(dat))

		if ret != 0 {
			*w.Status = 1
			return EbpfErrPolAdd
		}

		*w.Status = 0

	} else if w.Work == DpRemove {
		// Array map types need to be zeroed out first
		dat := new(polTact)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_pol_tact)
		C.llb_add_map_elem(C.LL_DP_POL_MAP, unsafe.Pointer(&key), unsafe.Pointer(dat))
		// This operation is unnecessary
		C.llb_del_map_elem(C.LL_DP_POL_MAP, unsafe.Pointer(&key))
		return 0
	}
	return 0
}

// DpPolAdd - routine to work on a ebpf policer add request
func (e *DpEbpfH) DpPolAdd(w *PolDpWorkQ) int {
	return e.DpPolMod(w)
}

// DpPolDel - routine to work on a ebpf policer delete request
func (e *DpEbpfH) DpPolDel(w *PolDpWorkQ) int {
	return e.DpPolMod(w)
}

// DpMirrMod - routine to work on a ebpf mirror modify request
func (e *DpEbpfH) DpMirrMod(w *MirrDpWorkQ) int {
	key := C.uint(w.HwMark)

	if w.Work == DpCreate {
		dat := new(mirrTact)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_mirr_tact)

		if w.MiBD != 0 {
			dat.ca.act_type = C.DP_SET_ADD_L2VLAN
		} else {
			dat.ca.act_type = C.DP_SET_RM_L2VLAN
		}

		la := (*l2VlanAct)(getPtrOffset(unsafe.Pointer(dat), C.sizeof_struct_dp_cmn_act))

		la.oport = C.ushort(w.MiPortNum)
		la.vlan = C.ushort(w.MiBD)

		ret := C.llb_add_map_elem(C.LL_DP_MIRROR_MAP, unsafe.Pointer(&key), unsafe.Pointer(dat))

		if ret != 0 {
			*w.Status = 1
			return EbpfErrMirrAdd
		}

		*w.Status = 0

	} else if w.Work == DpRemove {
		// Array map types need to be zeroed out first
		dat := new(mirrTact)
		C.memset(unsafe.Pointer(dat), 0, C.sizeof_struct_dp_mirr_tact)
		C.llb_add_map_elem(C.LL_DP_MIRROR_MAP, unsafe.Pointer(&key), unsafe.Pointer(dat))
		C.llb_del_map_elem(C.LL_DP_MIRROR_MAP, unsafe.Pointer(&key))
		return 0
	}
	return 0
}

// DpMirrAdd - routine to work on a ebpf mirror add request
func (e *DpEbpfH) DpMirrAdd(w *MirrDpWorkQ) int {
	return e.DpMirrMod(w)
}

// DpMirrDel - routine to work on a ebpf mirror delete request
func (e *DpEbpfH) DpMirrDel(w *MirrDpWorkQ) int {
	return e.DpMirrMod(w)
}
