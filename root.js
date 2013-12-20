
var lucos = require('_lucosjs');
lucos.waitFor('ready', function _rootloader() {
	var modules = lucos.bootdata.modules;
	if (window.location.pathname == "/") {
		
		if (window.navigator.standalone) {
				if (document.referrer) localStorage.removeItem("laststandaloneurl");
				var lasturl = localStorage.getItem("laststandaloneurl");
				if (lasturl) {
					
					// Function to redirect to relevant url
					var ii, ll, toolate = false, redirect = function () {
						if (toolate) return;
						location.href = lasturl;
					};
					
					// Find the module which the url belongs to
					for (ii=0, ll=modules.length; ii<ll; ii++) {
						var module = modules[ii];
						if (module.baseurl != lasturl) continue;
						if (module.requires.net && !lucos.detect.isOnline()) {
							
							// If the module requires network and there isn't any at the moment, wait up to a second for some, otherwise give up
							lucos.waitFor('online', redirect);
							window.setTimeout(function _giveUpOnRedirect() {
								toolate = true;
							}, 1000);
						} else {
							redirect();
						}
						break;
					}
				}
		}
		

		var preloadframes = [];
		lucos.listen('preload', function _gotPreloadMessage(message, preloadwindow) {
			var ii, ll;
			
			// TODO: if the status is 'ready' then it's okay to proceed, but there still should be some indication until 'done' is sent
			
			for (ii = 0, ll = preloadframes.length; ii<ll; ii++) {
				if (preloadframes[ii].window != preloadwindow) continue;
				if (preloadframes[ii].callback(message))
					preloadframes.splice(ii, 1);
				break;
			}
		});


		var linklist = document.getElementById('links'), ii, ll;
		while (linklist.hasChildNodes()) {
			linklist.removeChild(linklist.firstChild);
		}
		for (ii=0, ll=modules.length; ii<ll; ii++) {
			var module = modules[ii];
			(function _renderModule(module) {
				if (module.enabled === undefined) module.enabled = true;
				if (!module.enabled) return;
				var li = document.createElement('li');
				var link = document.createElement('a');
				var img = document.createElement("img");
				var loading, bar, moduleclass = 'module', loadingframe;
				
				link.setAttribute('href', module.baseurl);
				if (typeof module.img != 'string') module.img = module.baseurl + 'icon';
				img.src = module.img;
				img.setAttribute("alt", module.title);
				img.setAttribute("title", module.title);
				link.appendChild(img);
				
				if (module.requires.net) moduleclass += " networkonly";
				li.setAttribute('class', moduleclass);
				
				if (window.navigator.standalone) {
					link.addEventListener('click', function () {
						localStorage.setItem("laststandaloneurl", module.baseurl);
					}, true);
				}
				
				if (module.preload) {
					module.loaded = {};
					loading = document.createElement('span');
					loading.appendChild(img.cloneNode());
					loading.setAttribute("class", "loading");
					bar = document.createElement("div");
					bar.setAttribute("class", "loadingbar");
					bar.appendChild(document.createElement("span"));
					loading.appendChild(bar);
					loadingframe = document.createElement('iframe');
					if (typeof module.preload.src == 'string') loadingframe.src = module.preload.src;
					else loadingframe.src = module.baseurl+'preload';
					loadingframe.setAttribute("style", "height: 0; width: 0; display:none;");
					document.body.appendChild(loadingframe);
					preloadframes.push({
						window: loadingframe.contentWindow, 
						/**
						 * @returns boolean Whether the preload has finished
						 */
						callback: function _preloadcallback(message) {
							module.loaded[message.section] = message.result;
							moduleclass += ' '+message.section+'loaded';
							li.setAttribute('class', moduleclass);
							
							// TODO: allow customisation of what needs loaded from modules.json or from  /preload
							if (!module.loaded.manifest || !module.loaded.resources) return false;
							if (li) {
								li.removeChild(loading);
								li.appendChild(link);
							}
							return true;
						}
					});
					li.appendChild(loading);
				} else {
					li.appendChild(link);
				}
				linklist.appendChild(li);
			}) (module);
				
		}
		/*var links = linklist.getElementsByTagName('a');
		for (var ii=0, len = links.length; ii < len; ii++) {
			links[ii].addEventListener('click', function (event) {
				// Only handle left clicks
				if (event.button !== 0) return;
				
				document.body.setAttribute('class', document.body.getAttribute('class')+' loading');
			}, false);
		}*/
		// Remove the title and add nav bar instead
		document.body.removeChild(document.getElementById('title'));
		lucos.addNavBar('Home');
	}
});

