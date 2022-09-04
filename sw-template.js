const STATIC_CACHE = 'static-v1';
// Array generated in Dockerfile based on listing all files
const localUrls = [
	"/",
];
const crossDomainUrls = [
	"https://seinn.l42.eu/logo.jpg",
	"https://notes.l42.eu/icon.png",
	"https://monitoring.l42.eu/icon",
	"https://am.l42.eu/icon.png",
	"https://app.tfluke.uk/img/icon.png",
	"https://contacts.l42.eu/resources/logo-highres.png",
	"https://loganne.l42.eu/icon",
];
async function install() {
	try {
		const cache = await caches.open(STATIC_CACHE);
		await cache.addAll(localUrls);

		// `addAll` doesn't work for URLs which need a `no-cors` request
		// Instead need to fetch them individually and call `put`
		Promise.all(crossDomainUrls.map(async url => {
			const request = new Request(url, {mode: 'no-cors'})
			const response = await fetch(request);
			await cache.put(request, response);
		}));
	} catch (error) {
		console.error("Failed to cache resources:", error.message);
	}
}
self.addEventListener('install', event => {
	event.waitUntil(install());
});

self.addEventListener('fetch', event => {
	event.respondWith((async () => {
		const cachedResponse = await caches.match(event.request);
		if (cachedResponse) return cachedResponse;
		else return fetch(event.request);
	})());
});