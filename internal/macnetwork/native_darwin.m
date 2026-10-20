//go:build darwin && cgo

#import <Foundation/Foundation.h>
#include <CFNetwork/CFNetwork.h>
#include <SystemConfiguration/SystemConfiguration.h>
#include <ifaddrs.h>
#include <net/if.h>
#include <net/if_dl.h>
#include <stdatomic.h>
#include <pthread.h>
#include <string.h>
#include <errno.h>
#include <time.h>
#include "native_darwin.h"

struct ng_request {
    atomic_int cancelled;
    int done;
    CFArrayRef proxies;
    CFErrorRef error;
};

static atomic_uint_fast64_t ng_network_generation;
static pthread_mutex_t ng_watch_mutex=PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t ng_watch_condition=PTHREAD_COND_INITIALIZER;
static int ng_watch_status;
static unsigned ng_watch_delay;
static struct timespec ng_watch_retry;
static CFRunLoopRef ng_watch_loop;

static void ng_changed(SCDynamicStoreRef store,CFArrayRef keys,void *info) {
    atomic_fetch_add(&ng_network_generation,1);
}
static void ng_watch_signal(int status) {
    pthread_mutex_lock(&ng_watch_mutex);
    ng_watch_status=status;
    atomic_fetch_add(&ng_network_generation,1);
    if (status<0) {
        ng_watch_delay=ng_watch_delay ? MIN(ng_watch_delay*2,30U) : 1;
        clock_gettime(CLOCK_MONOTONIC,&ng_watch_retry);
        ng_watch_retry.tv_sec+=ng_watch_delay;
    } else if (status==1) ng_watch_delay=0;
    pthread_cond_broadcast(&ng_watch_condition);
    pthread_mutex_unlock(&ng_watch_mutex);
}
static void *ng_watch_thread(void *unused) {
    @autoreleasepool {
        SCDynamicStoreContext context={0,NULL,NULL,NULL,NULL};
        SCDynamicStoreRef store=SCDynamicStoreCreate(NULL,CFSTR("NetGAP.Changes"),ng_changed,&context);
        if (!store) { ng_watch_signal(-1); return NULL; }
        CFArrayRef patterns=(CFArrayRef)@[@"State:/Network/.*",@"Setup:/Network/.*"];
        if (!SCDynamicStoreSetNotificationKeys(store,NULL,patterns)) { CFRelease(store); ng_watch_signal(-1); return NULL; }
        CFRunLoopSourceRef source=SCDynamicStoreCreateRunLoopSource(NULL,store,0);
        if (!source) { CFRelease(store); ng_watch_signal(-1); return NULL; }
        CFRunLoopAddSource(CFRunLoopGetCurrent(),source,kCFRunLoopDefaultMode);
        pthread_mutex_lock(&ng_watch_mutex);
        ng_watch_loop=CFRetain(CFRunLoopGetCurrent());
        pthread_mutex_unlock(&ng_watch_mutex);
        ng_watch_signal(1);
        CFRunLoopRun();
        pthread_mutex_lock(&ng_watch_mutex);
        CFRelease(ng_watch_loop); ng_watch_loop=NULL;
        pthread_mutex_unlock(&ng_watch_mutex);
        CFRunLoopSourceInvalidate(source); CFRelease(source); CFRelease(store);
        ng_watch_signal(-1);
    }
    return NULL;
}

static void ng_watch_invalidate(void) {
    pthread_mutex_lock(&ng_watch_mutex);
    atomic_fetch_add(&ng_network_generation,1);
    if (ng_watch_status==1 && ng_watch_loop) {
        ng_watch_status=2;
        CFRunLoopStop(ng_watch_loop);
        CFRunLoopWakeUp(ng_watch_loop);
    }
    pthread_mutex_unlock(&ng_watch_mutex);
}

/* Restart the watcher through the same invalidation path used after native
   SystemConfiguration failures. Kept internal to the macnetwork package. */
void ng_network_watcher_restart(void) { ng_watch_invalidate(); }

static void ng_watch_start(void) {
    pthread_t thread;
    if (pthread_create(&thread,NULL,ng_watch_thread,NULL)!=0) ng_watch_signal(-1);
    else pthread_detach(thread);
}
static BOOL ng_watch_ready(void) {
    /* Claim startup under the mutex, then create the thread outside it: a
       pthread_create failure reports through ng_watch_signal using this lock. */
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC,&now);
    pthread_mutex_lock(&ng_watch_mutex);
    BOOL start=ng_watch_status==0 || (ng_watch_status<0 &&
        (now.tv_sec>ng_watch_retry.tv_sec || (now.tv_sec==ng_watch_retry.tv_sec && now.tv_nsec>=ng_watch_retry.tv_nsec)));
    if (start) ng_watch_status=2;
    pthread_mutex_unlock(&ng_watch_mutex);
    if (start) ng_watch_start();
    struct timespec deadline;
    clock_gettime(CLOCK_REALTIME,&deadline);
    deadline.tv_sec+=5;
    pthread_mutex_lock(&ng_watch_mutex);
    while (ng_watch_status==2) {
        int error=pthread_cond_timedwait(&ng_watch_condition,&ng_watch_mutex,&deadline);
        if (error!=0) { pthread_mutex_unlock(&ng_watch_mutex); return NO; }
    }
    BOOL ready=ng_watch_status==1;
    pthread_mutex_unlock(&ng_watch_mutex);
    return ready;
}

static char *ng_json(id object) {
    NSError *error=nil;
    NSData *data=[NSJSONSerialization dataWithJSONObject:object options:NSJSONWritingSortedKeys error:&error];
    if (!data) return strdup("{\"error\":\"Cannot serialize macOS network result\"}");
    return strndup(data.bytes,data.length);
}
static char *ng_error(NSString *message) { return ng_json(@{@"error":message ?: @"macOS network operation failed"}); }
static id ng_normalize(id object) {
    if ([object isKindOfClass:[NSDictionary class]]) {
        NSMutableDictionary *result=[NSMutableDictionary dictionary];
        for (NSString *key in object) result[key]=ng_normalize(object[key]);
        return result;
    }
    if ([object isKindOfClass:[NSArray class]]) {
        NSMutableArray *result=[NSMutableArray array];
        for (id value in object) [result addObject:ng_normalize(value)];
        return result;
    }
    if ([object isKindOfClass:[NSData class]]) return [object base64EncodedStringWithOptions:0];
    if ([object isKindOfClass:[NSDate class]]) return @([object timeIntervalSince1970]);
    return object;
}
static NSString *ng_plist(id object) {
    if (!object) return nil;
    NSData *data=[NSJSONSerialization dataWithJSONObject:ng_normalize(object) options:NSJSONWritingSortedKeys error:NULL];
    return [data base64EncodedStringWithOptions:0];
}
static BOOL ng_enabled(id settings) {
    if ([settings isKindOfClass:[NSArray class]]) {
        for (id value in settings) { if (ng_enabled(value)) return YES; }
        return NO;
    }
    if (![settings isKindOfClass:[NSDictionary class]]) return NO;
    for (NSString *key in @[@"HTTPEnable",@"HTTPSEnable",@"SOCKSEnable",@"ProxyAutoConfigEnable",@"ProxyAutoDiscoveryEnable"]) {
        if ([settings[key] respondsToSelector:@selector(boolValue)] && [settings[key] boolValue]) return YES;
    }
    for (id value in [settings allValues]) {
        if (ng_enabled(value)) return YES;
    }
    return NO;
}

ng_request *ng_request_new(void) {
    ng_request *r=calloc(1,sizeof(ng_request));
    if (r) atomic_init(&r->cancelled,0);
    return r;
}
void ng_request_cancel(ng_request *r) { atomic_store(&r->cancelled,1); }
void ng_request_free(ng_request *r) {
    if (r->proxies) CFRelease(r->proxies);
    if (r->error) CFRelease(r->error);
    free(r);
}

char *ng_proxy_settings(void) {
    @autoreleasepool {
        CFDictionaryRef settings=CFNetworkCopySystemProxySettings();
        if (!settings) return ng_error(@"Cannot read macOS system proxy settings");
        NSString *state=ng_plist((id)settings);
        char *result=state ? ng_json(@{@"configured":@(ng_enabled((NSDictionary *)settings)),@"state":state}) : ng_error(@"Cannot serialize system proxy settings");
        CFRelease(settings);
        return result;
    }
}

static void ng_proxy_callback(void *info,CFArrayRef proxies,CFErrorRef error) {
    ng_request *r=info;
    if (proxies) r->proxies=CFRetain(proxies);
    if (error) r->error=CFRetain(error);
    r->done=1;
}

static CFArrayRef ng_auto(ng_request *r,CFDictionaryRef entry,CFURLRef target) {
    if (r->proxies) { CFRelease(r->proxies); r->proxies=NULL; }
    if (r->error) { CFRelease(r->error); r->error=NULL; }
    r->done=0;
    CFStreamClientContext context={0,r,NULL,NULL,NULL};
    CFStringRef type=CFDictionaryGetValue(entry,kCFProxyTypeKey);
    CFRunLoopSourceRef source=NULL;
    if (CFEqual(type,kCFProxyTypeAutoConfigurationURL)) {
        CFURLRef url=CFDictionaryGetValue(entry,kCFProxyAutoConfigurationURLKey);
        if (url) source=CFNetworkExecuteProxyAutoConfigurationURL(url,target,ng_proxy_callback,&context);
    } else if (CFEqual(type,kCFProxyTypeAutoConfigurationJavaScript)) {
        CFStringRef script=CFDictionaryGetValue(entry,kCFProxyAutoConfigurationJavaScriptKey);
        if (script) source=CFNetworkExecuteProxyAutoConfigurationScript(script,target,ng_proxy_callback,&context);
    }
    if (!source) return NULL;
    CFRunLoopRef loop=CFRunLoopGetCurrent();
    CFRunLoopAddSource(loop,source,kCFRunLoopDefaultMode);
    CFAbsoluteTime deadline=CFAbsoluteTimeGetCurrent()+25;
    while (!r->done && !atomic_load(&r->cancelled) && CFAbsoluteTimeGetCurrent()<deadline) {
        CFRunLoopRunInMode(kCFRunLoopDefaultMode,0.05,false);
    }
    CFRunLoopSourceInvalidate(source);
    CFRunLoopRemoveSource(loop,source,kCFRunLoopDefaultMode);
    CFRelease(source);
    if (!r->done || r->error || !r->proxies) return NULL;
    return CFRetain(r->proxies);
}

static NSString *ng_paths(ng_request *r,CFArrayRef proxies,CFURLRef target,NSMutableArray *paths,int depth) {
    if (depth>2) return @"Recursive proxy auto-configuration result";
    for (CFIndex i=0;i<CFArrayGetCount(proxies);i++) {
        if (atomic_load(&r->cancelled)) return @"Proxy rule resolution cancelled";
        CFDictionaryRef entry=CFArrayGetValueAtIndex(proxies,i);
        CFStringRef type=CFDictionaryGetValue(entry,kCFProxyTypeKey);
        if (!type) return @"Missing system proxy type";
        if (CFEqual(type,kCFProxyTypeAutoConfigurationURL) || CFEqual(type,kCFProxyTypeAutoConfigurationJavaScript)) {
            CFArrayRef resolved=ng_auto(r,entry,target);
            if (!resolved) return @"PAC/WPAD download or execution failed; no implicit direct fallback";
            NSString *error=ng_paths(r,resolved,target,paths,depth+1);
            CFRelease(resolved);
            if (error) return error;
            continue;
        }
        NSString *scheme=nil;
        if (CFEqual(type,kCFProxyTypeNone)) scheme=@"direct";
        else if (CFEqual(type,kCFProxyTypeHTTP)) scheme=@"http";
        // CFNetwork's secure-web proxy is an HTTP CONNECT proxy for HTTPS
        // destinations; this type does not imply TLS to the proxy itself.
        else if (CFEqual(type,kCFProxyTypeHTTPS)) scheme=@"http";
        else if (CFEqual(type,kCFProxyTypeSOCKS)) scheme=@"socks5";
        else return @"Unsupported macOS system proxy type";
        NSMutableDictionary *path=[NSMutableDictionary dictionaryWithObject:scheme forKey:@"scheme"];
        if (![scheme isEqualToString:@"direct"]) {
            id host=(id)CFDictionaryGetValue(entry,kCFProxyHostNameKey);
            id port=(id)CFDictionaryGetValue(entry,kCFProxyPortNumberKey);
            if (![host isKindOfClass:[NSString class]] || ![port respondsToSelector:@selector(intValue)]) return @"Invalid macOS proxy endpoint";
            path[@"host"]=host; path[@"port"]=port;
            id user=(id)CFDictionaryGetValue(entry,kCFProxyUsernameKey);
            id password=(id)CFDictionaryGetValue(entry,kCFProxyPasswordKey);
            if ([user isKindOfClass:[NSString class]]) path[@"username"]=user;
            if ([password isKindOfClass:[NSString class]]) path[@"password"]=password;
        }
        [paths addObject:path];
        if (paths.count>64) return @"Too many system proxy candidates";
    }
    return nil;
}

char *ng_proxy_routes(ng_request *r,const char *target) {
    @autoreleasepool {
        CFURLRef url=CFURLCreateWithBytes(NULL,(const UInt8 *)target,strlen(target),kCFStringEncodingUTF8,NULL);
        if (!url) return ng_error(@"Invalid proxy target URL");
        CFDictionaryRef settings=CFNetworkCopySystemProxySettings();
        if (!settings) { CFRelease(url); return ng_error(@"Cannot read macOS proxy rules"); }
        CFArrayRef proxies=CFNetworkCopyProxiesForURL(url,settings);
        CFRelease(settings);
        if (!proxies) { CFRelease(url); return ng_error(@"System returned no proxy rules"); }
        NSMutableArray *paths=[NSMutableArray array];
        NSString *error=ng_paths(r,proxies,url,paths,0);
        CFRelease(proxies); CFRelease(url);
        if (error) return ng_error(error);
        if (!paths.count) return ng_error(@"System returned no proxy candidates");
        return ng_json(@{@"paths":paths});
    }
}

char *ng_network_state(void) {
    @autoreleasepool {
        if (!ng_watch_ready()) return ng_error(@"Cannot subscribe to macOS network changes");
        SCDynamicStoreRef store=SCDynamicStoreCreate(NULL,CFSTR("NetGAP"),NULL,NULL);
        if (!store) { ng_watch_invalidate(); return ng_error(@"Cannot open macOS network state; subscription will retry"); }
        uint64_t before=atomic_load(&ng_network_generation);
        CFArrayRef patterns=(CFArrayRef)@[@"State:/Network/.*"];
        CFDictionaryRef values=SCDynamicStoreCopyMultiple(store,NULL,patterns);
        uint64_t after=atomic_load(&ng_network_generation);
        NSString *state=values && before==after ? ng_plist(@{@"generation":@(after),@"values":(id)values}) : nil;
        if (!values) ng_watch_invalidate();
        char *result=state ? ng_json(@{@"state":state}) : ng_error(@"Cannot capture macOS network state");
        if (values) CFRelease(values); CFRelease(store);
        return result;
    }
}

char *ng_interface_binding(const char *name,int family) {
    @autoreleasepool {
        NSString *bsd=[NSString stringWithUTF8String:name];
        CFArrayRef interfaces=SCNetworkInterfaceCopyAll();
        NSString *identity=nil;
        if (interfaces) {
            for (CFIndex i=0;i<CFArrayGetCount(interfaces);i++) {
                SCNetworkInterfaceRef interface=(SCNetworkInterfaceRef)CFArrayGetValueAtIndex(interfaces,i);
                if ([(id)SCNetworkInterfaceGetBSDName(interface) isEqual:bsd]) {
                    identity=[NSString stringWithFormat:@"%@/%@/%@",bsd,(id)SCNetworkInterfaceGetInterfaceType(interface) ?: @"",(id)SCNetworkInterfaceGetHardwareAddressString(interface) ?: @""];
                    break;
                }
            }
        }
        // Virtual interfaces may not be listed by SystemConfiguration.
        struct ifaddrs *addresses=NULL;
        NSString *link=nil;
        if (getifaddrs(&addresses)==0) {
            for (struct ifaddrs *a=addresses;a;a=a->ifa_next) {
                if (strcmp(a->ifa_name,name)!=0 || !a->ifa_addr || a->ifa_addr->sa_family!=AF_LINK) continue;
                struct sockaddr_dl *dl=(struct sockaddr_dl *)a->ifa_addr;
                struct if_data *data=(struct if_data *)a->ifa_data;
                if (!identity) identity=[NSString stringWithFormat:@"%@/%u/%u",bsd,dl->sdl_index,dl->sdl_type];
                if (data) link=[NSString stringWithFormat:@"%ld.%06d",(long)data->ifi_lastchange.tv_sec,(int)data->ifi_lastchange.tv_usec];
            }
            freeifaddrs(addresses);
        }
        if (interfaces) CFRelease(interfaces);
        if (!identity || !link) return ng_error(@"Cannot identify macOS route interface");
        SCDynamicStoreRef store=SCDynamicStoreCreate(NULL,CFSTR("NetGAP.Binding"),NULL,NULL);
        if (!store) return ng_error(@"Cannot read interface network identity");
        NSString *suffix=family==6 ? @"IPv6" : @"IPv4";
        NSString *pattern=[NSString stringWithFormat:@"State:/Network/Service/.*/%@",suffix];
        CFDictionaryRef values=SCDynamicStoreCopyMultiple(store,NULL,(CFArrayRef)@[pattern]);
        if (!values) { CFRelease(store); return ng_error(@"Cannot capture interface network identity"); }
        NSMutableDictionary *network=[NSMutableDictionary dictionaryWithObject:link forKey:@"linkChanged"];
        for (NSString *key in (NSDictionary *)values) {
            NSDictionary *value=((NSDictionary *)values)[key];
            if (![value[@"InterfaceName"] isEqual:bsd]) continue;
            NSMutableDictionary *selected=[NSMutableDictionary dictionary];
            for (NSString *field in @[@"NetworkSignature",@"Router",@"ServerAddress"]) { if (value[field]) selected[field]=value[field]; }
            network[key]=selected;
        }
        NSString *state=ng_plist(network);
        char *result=state ? ng_json(@{@"identity":identity,@"network":state}) : ng_error(@"Cannot serialize interface identity");
        if (values) CFRelease(values); CFRelease(store);
        return result;
    }
}
