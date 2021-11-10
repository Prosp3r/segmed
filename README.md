# SEGMED IMAGE GALLERY


### Setup

This part has been made as easy as possible.
I have built binaries for all major platforms and systems architecture
So all that's needed is to: 

1. navigate through the production folder and copy the appropriate main binary to the home folder and 

    e.g Lunix: from the home directory run 

    ```cp builds/production/linux/386/main .```
    
    ```./main```

    Visit the server through the browser on port 8080
    e.g. https://server-ip:8080

    Be sure to have websockets permitted on the firewall if you have one running.

2. Run ```./main``` on the terminal

E.g: On Mac go to production/darwin/amd64 copy the file 'main' to the project home directory where we have other asset folders and run ./main

Same for Linux, Windows, Android, etc.


### How it works
This piece of code reads all images in the folder /assets/medimages/ into memory in real time(i.e: if a new image is put in after the program starts, it'll read it into the page vies immdiately).

- It stores a record of everything in a segmed.JSON file (Our makeshift database) in the home directory.
- It also stores all meta-data extracted from the images in the JSON file.
- It stores the tag status of any tagged image in the .JSON file representation.

- JSON file was chosen for ease of proof and portability in this case.

- Deployment could have been achieve in a number of ways(including docker containers) but for ease in this case, I chose to generate binaries for as many platforms as possible using a simple script in buildProd.sh, this enables the proof to be achieved in almost any platform without complications. or Installation requirements. 
Since the work is already done, We just need to copy and paste the appropriate platform's binary to home directory.

- 

