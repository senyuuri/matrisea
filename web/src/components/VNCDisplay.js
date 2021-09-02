import RFB from '@novnc/novnc/core/rfb';
import { useEffect } from 'react';

function VNCDisplay(props){

    var rfb;
    // When this function is called we have
    // successfully connected to a server
    function connectedToServer(e) {
        console.log("Connected");
    }

    // This function is called when we are disconnected
    function disconnectedFromServer(e) {
        if (e.detail.clean) {
            console.log("Disconnected");
        } else {
            console.log("Something went wrong, connection is closed");
        }
    }

    useEffect(() => {
        // Creating a new RFB object will start a new connection
        rfb = new RFB(document.getElementById('vnc-canvas'), props.url, {
            wsProtocols: ['binary', 'base64'],
        });

        // Add listeners to important events from the RFB module
        rfb.addEventListener("connect",  connectedToServer);
        rfb.addEventListener("disconnect", disconnectedFromServer);

        // Set parameters that can be changed on an active connection
        rfb.viewOnly = false;
        rfb.scaleViewport = true;
        rfb.showDotCursor = true;
    }, []);

    return (
        <div id="vnc-canvas"></div>
    )
}

export default VNCDisplay