const servicesUrl= "/config.json"

let currentHost = window.location.hostname;
let tabContainer = document.getElementById('tabContainer');
let contentWrapper = document.getElementById('contentWrapper');

let allTabs = [];
let allContents = [];
let allIframes = [];
let serviceHashes = [];

function activateTab(index) {
    allTabs.forEach(tab => tab.classList.remove('active'));
    allContents.forEach(cont => cont.classList.remove('active'));

    if (index >= 0 && index < allTabs.length) {
        let iframeEl = allIframes[index];
        if (iframeEl && !iframeEl.src) {
            iframeEl.src = iframeEl.dataset.src;
        }

        allTabs[index].classList.add('active');
        allContents[index].classList.add('active');
        window.location.hash = serviceHashes[index];
    }
}

function createTab(serviceName, serviceUrl, hashValue) {
    let tabEl = document.createElement('a');
    tabEl.className = 'tab';
    tabEl.textContent = serviceName;
    tabEl.href = '#' + encodeURIComponent(hashValue);

    let extLink = document.createElement('a');
    extLink.href = serviceUrl;
    extLink.title=`Открыть ${serviceName} во внешней вкладке`
    extLink.target = '_blank';
    extLink.textContent = '🔗';
    extLink.className = 'tab-ext-link';

    tabEl.appendChild(extLink);

    let contentEl = document.createElement('div');
    contentEl.className = 'content-container';
    let iframeEl = document.createElement('iframe');
    iframeEl.dataset.src = serviceUrl;
    iframeEl.loading = 'lazy';
    contentEl.appendChild(iframeEl);

    tabContainer.appendChild(tabEl);
    contentWrapper.appendChild(contentEl);

    allTabs.push(tabEl);
    allContents.push(contentEl);
    allIframes.push(iframeEl);
}


function tryActivateTabFromHash() {
    let rawHashValue = window.location.hash ? window.location.hash.substring(1) : '';
    let decodedHashValue = decodeURIComponent(rawHashValue);
    let idx = serviceHashes.indexOf(decodedHashValue);
    if (idx !== -1) {
        activateTab(idx);
    } else {
        activateTab(0);
    }
}

fetch(servicesUrl)
    .then(response => response.json())
    .then(config => {
        let internalHostname = config.internalHostname;
        let services = config.services;

        if (internalHostname === currentHost){
            services.forEach(service => {
                serviceHashes.push(service.internalHostname);
                let url = `http://${service.internalHostname}:${service.internalPort}`;
                createTab(service.name, url, service.internalHostname);
            });
        }
        else {
            services.forEach(service => {
                serviceHashes.push(service.internalHostname);
                let url = `https://${currentHost}:${service.externalPort}`;
                createTab(service.name, url, service.internalHostname);
            });
        }

        tryActivateTabFromHash();
    })
    .catch(err => {
        console.error(`Error fetching ${servicesUrl}:`, err);
    });


window.addEventListener('hashchange', tryActivateTabFromHash);
