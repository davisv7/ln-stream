var toggleStatus = true; // Initial value of the toggle status

// Initialize the indicator and toggleStatus values using an API request
fetch('/get-status')
    .then(response => response.json())
    .then(data => {
        updateIndicator(data.isRoutineRunning);
        toggleStatus = data.isRoutineRunning;
        updateToggleButton();
    })
    .catch(error => {
        // Handle error if the request fails
        console.error("Failed to retrieve graph update status:", error);
    });

function fetchToggleUpdates() {
    fetch('/toggle-updates')
        .then(response => {
            if (response.ok) {
                toggleStatus = !toggleStatus; // Reverse the toggle status
                updateToggleButton();
                if (toggleStatus) {
                    alert("Graph updates enabled.");
                } else {
                    alert("Graph updates disabled.");
                }
            } else {
                alert("Failed to toggle graph updates.");
            }
            return response.json(); // Parse the response as JSON
        })
        .then(data => {
            toggleStatus = data.isRoutineRunning;
            updateIndicator(data.isRoutineRunning); // Update the indicator based on the returned value
        });
}

function updateToggleButton() {
    var toggleButton = document.getElementById("toggleButton");

    if (toggleStatus) {
        toggleButton.textContent = "Disable Graph Updates";
        toggleButton.style.backgroundColor = "#007bff";
    } else {
        toggleButton.textContent = "Enable Graph Updates";
        toggleButton.style.backgroundColor = "#dc3545";
    }
}

function updateIndicator(isRunning) {
    var indicator = document.getElementById("indicator");

    if (isRunning) {
        indicator.classList.add("green");
        indicator.classList.remove("red");
    } else {
        indicator.classList.add("red");
        indicator.classList.remove("green");
    }
}

function disableButtons() {
    document.getElementById("resetButton").disabled = true;
    document.getElementById("toggleButton").disabled = true;
}

function enableButtons() {
    document.getElementById("resetButton").disabled = false;
    document.getElementById("toggleButton").disabled = false;
}

document.getElementById("localButton").onclick = function () {
    alert("Graph reset initiated.");
    disableButtons(); // Disable buttons when the request is initiated
    fetch('/load-local-snapshot',{cache: "no-store"}).then(response => {
        if (response.ok) {
            alert("Graph reset OK.");
        } else {
            alert("Failed to initiate graph reset.");
        }
        enableButtons(); // Enable buttons when the request is completed
    });
};

document.getElementById("resetButton").onclick = function () {
    alert("Graph reset initiated.");
    disableButtons(); // Disable buttons when the request is initiated
    fetch('/reset-graph').then(response => {
        if (response.ok) {
            alert("Graph reset OK.");
        } else {
            alert("Failed to initiate graph reset.");
        }
        enableButtons(); // Enable buttons when the request is completed
    });
};

document.getElementById("toggleButton").onclick = function () {
    fetchToggleUpdates();
};

updateToggleButton();