#include <gio/gio.h>
#include <stdlib.h>
const char *ng_backend(void);
char *ng_snapshot(int *configured);
char **ng_lookup(const char *target, char **error);
