#include <stdlib.h>

typedef struct ng_request ng_request;
ng_request *ng_request_new(void);
void ng_request_cancel(ng_request *request);
void ng_request_free(ng_request *request);
char *ng_proxy_settings(void);
char *ng_proxy_routes(ng_request *request, const char *target);
char *ng_network_state(void);
char *ng_interface_binding(const char *name, int family);
