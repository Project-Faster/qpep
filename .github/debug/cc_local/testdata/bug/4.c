void f(void *p, void *q);

int main() {
	void *p;
	f((_Atomic(char) *)p, p);
}
